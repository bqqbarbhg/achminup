package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var downloadBase string
var srcBase string
var dstBase string
var serveBase string
var layersApiUri string

var reUUID = regexp.MustCompile("[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}")
var reRotation = regexp.MustCompile("Rotation\\s*:\\s*(\\d+)")

var requestID int32
var processID int32

var serveFileMutex = &sync.Mutex{}

func authenticate(r *http.Request) (identity string, err error) {
	url := layersApiUri + "/o/oauth2/userinfo"

	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", r.Header.Get("Authorization"))
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", errors.New("OIDC responded with non-200 status")
	}

	decoder := json.NewDecoder(resp.Body)
	data := make(map[string]interface{})
	err = decoder.Decode(&data)
	if err != nil {
		return "", err
	}

	uid := data["sub"]
	strid, ok := uid.(string)
	if !ok {
		return "", errors.New("OIDC did not return an user id")
	}

	return strid, nil
}

type apiData struct {
	ID        string
	User      string
	Format    string
	Extension string
}

func checkOwnerFile(owner string, context func(bool)) bool {
	serveFileMutex.Lock()

	_, err := os.Stat(owner)
	exists := err == nil

	context(exists)

	serveFileMutex.Unlock()

	return exists
}

var rotationAvconvArguments = map[int][]string{
	0:   {},
	90:  {"-vf", "transpose=1"},
	180: {"-vf", "vflip,hflip"},
	270: {"-vf", "transpose=3"},
}

func transcode(src string, dst string, rotation int, logger *log.Logger, arguments ...string) error {
	args := []string{
		// Input file
		"-i", src,

		// Overwrite
		"-y",

		// Convert audio: copy
		"-c:a", "copy",

		// Convert video: h264
		"-c:v", "h264",

		// Log level
		"-v", "warning",
	}

	// Rotation compensation
	rotationArgs := rotationAvconvArguments[rotation]
	if len(rotationArgs) > 0 {
		args = append(args, rotationArgs...)
	}

	// Custom arguments
	if len(arguments) > 0 {
		args = append(args, arguments...)
	}

	// Output file
	args = append(args, dst)

	transcodeCmd := exec.Command("avconv", args...)
	logger.Printf("> %s\n", strings.Join(transcodeCmd.Args, " "))
	transcodeOutput, err := transcodeCmd.CombinedOutput()
	logger.Printf("Output:\n%s", string(transcodeOutput))

	if err != nil {
		logger.Printf("Failed to transcode%s\n", err.Error())
	}

	return err
}

type processFunc func(string, string, string, string, *log.Logger) error

func processVideo(src string, dst string, srv string, owner string, logger *log.Logger) error {

	rotationCmd := exec.Command("exiftool", "-Rotation", src)
	logger.Printf("> %s\n", strings.Join(rotationCmd.Args, " "))
	rotationOutput, err := rotationCmd.Output()

	rotation := 0

	if err == nil {
		matches := reRotation.FindSubmatch(rotationOutput)
		if len(matches) < 2 {
			rotation, err = strconv.Atoi(string(matches[1]))
			if err != nil {
				logger.Printf("Failed to parse rotation: %s\n", err.Error())
			} else {
				logger.Printf("Found rotation %d\n", rotation)
			}
		} else {
			logger.Printf("Did not find rotation\n")
		}
	} else {
		logger.Printf("Failed to extract rotation: %s\n", err.Error())
	}

	logger.Println("Transcoding temporary low-quality video")
	err = transcode(src, dst, rotation, logger, "-preset", "ultrafast")
	if err == nil {

		exists := checkOwnerFile(owner, func(exists bool) {

			if exists {
				err = os.Rename(dst, srv)
				if err != nil {
					logger.Printf("Failed to rename low-quality video: %s", err.Error())
				} else {
					logger.Printf("Renamed low-quality video as %s", srv)
				}
			} else {
				_ = os.Remove(src)
				_ = os.Remove(dst)
				_ = os.Remove(srv)
			}

		})

		if !exists {
			return errors.New("File deleted during processing")
		}

	}

	logger.Println("Transcoding high-quality video")
	err = transcode(src, dst, rotation, logger, "-qscale", "1")
	if err == nil {

		exists := checkOwnerFile(owner, func(exists bool) {

			if exists {
				err = os.Rename(dst, srv)
				if err != nil {
					logger.Printf("Failed to rename high-quality video: %s", err.Error())
				} else {
					logger.Printf("Renamed high-quality video as %s", srv)
				}
			} else {
				_ = os.Remove(src)
				_ = os.Remove(dst)
				_ = os.Remove(srv)
			}

		})
		if !exists {
			return errors.New("File deleted during processing")
		}
	}

	err = os.Remove(src)
	if err != nil {
		return err
	}

	return nil
}

func processThumbnail(src string, dst string, srv string, owner string, logger *log.Logger) error {

	// TODO: Moving between mounts?
	exists := checkOwnerFile(owner, func(exists bool) {
		if exists {
			err := os.Rename(src, srv)
			if err == nil {
				logger.Printf("Moved thumbnail %s -> %s", src, srv)
			}
		} else {
			_ = os.Remove(src)
			_ = os.Remove(srv)
		}
	})

	if !exists {
		return errors.New("File deleted during processing")
	}

	return nil
}

var processFuncs = map[string]processFunc{
	"videos":     processVideo,
	"thumbnails": processThumbnail,
}

func doProcessing(procType string, src string, dst string, srv string, owner string) {
	buffer := new(bytes.Buffer)
	procID := atomic.AddInt32(&processID, 1)
	procIDString := fmt.Sprintf("p-%d: ", int(procID))
	logger := log.New(buffer, procIDString, log.LstdFlags)

	logger.Printf("Starting processing for '%s'\n", procType)
	logger.Printf("src: %s\n", src)
	logger.Printf("dst: %s\n", dst)
	logger.Printf("srv: %s\n", srv)

	startTime := time.Now().UTC()
	err := processFuncs[procType](src, dst, srv, owner, logger)
	endTime := time.Now().UTC()

	duration := endTime.Sub(startTime)

	if err != nil {
		logger.Printf("Failed (%.2fs): %s", duration.Seconds(), err.Error())
	} else {
		logger.Printf("Succeeded (%.2fs)", duration.Seconds())
	}

	log.Printf("Output for process %d\n%s\n", procID, buffer.String())
}

func uploadHandler(w http.ResponseWriter, r *http.Request, api *apiData, logger *log.Logger) (error, int) {

	logger.Printf("Uploading %s%s to '%s'", api.ID, api.Extension, api.Format)

	ownerSrcPath := path.Join(downloadBase, api.Format, api.ID+".owner.txt")
	ownerDstPath := path.Join(serveBase, api.Format, api.ID+".owner.txt")

	if _, err := os.Stat(ownerDstPath); err == nil {
		return errors.New("Owner file already exists"), http.StatusForbidden
	} else {
		logger.Printf("Owner stat: %s", err.Error())
	}

	ownerFile, err := os.Create(ownerSrcPath)
	if err != nil {
		return err, http.StatusInternalServerError
	}
	logger.Printf("Created owner %s", ownerSrcPath)

	ownerFile.WriteString(api.User)

	err = ownerFile.Close()
	if err != nil {
		return err, http.StatusInternalServerError
	}

	outDownloadPath := path.Join(downloadBase, api.Format, api.ID+api.Extension)
	outSrcPath := path.Join(srcBase, api.Format, api.ID+api.Extension)
	outDstPath := path.Join(dstBase, api.Format, api.ID+api.Extension)
	outServePath := path.Join(serveBase, api.Format, api.ID+api.Extension)

	outFile, err := os.Create(outDownloadPath)
	if err != nil {
		return err, http.StatusInternalServerError
	}

	_, err = io.Copy(outFile, r.Body)
	if err != nil {
		return err, http.StatusInternalServerError
	}

	err = outFile.Close()
	if err != nil {
		return err, http.StatusInternalServerError
	}
	logger.Printf("Downloaded resource %s", outDownloadPath)

	serveFileMutex.Lock()
	err = os.Rename(ownerSrcPath, ownerDstPath)
	serveFileMutex.Unlock()

	if err != nil {
		return err, http.StatusInternalServerError
	}
	logger.Printf("Moved owner to %s", ownerDstPath)

	err = os.Rename(outDownloadPath, outSrcPath)
	if err != nil {
		return err, http.StatusInternalServerError
	}
	logger.Printf("Moved resource to %s", outSrcPath)

	go doProcessing(api.Format, outSrcPath, outDstPath, outServePath, ownerDstPath)

	url := layersApiUri + path.Join(os.Getenv("ACHMINUP_PATH"), api.Format, api.ID+api.Extension)
	fmt.Fprintf(w, "%s\n", url)

	return nil, http.StatusOK
}

func deleteHandler(w http.ResponseWriter, r *http.Request, api *apiData, logger *log.Logger) (error, int) {
	logger.Printf("Deleting %s%s from '%s'", api.ID, api.Extension, api.Format)

	ownerPath := path.Join(serveBase, api.Format, api.ID+".owner.txt")
	ownPath := path.Join(serveBase, api.Format, api.ID+api.Extension)

	_, err := io.Copy(ioutil.Discard, r.Body)
	if err != nil {
		return err, http.StatusInternalServerError
	}

	var outErr error = nil
	var outStatus int

	_ = checkOwnerFile(ownerPath, func(exists bool) {

		if !exists {
			logger.Printf("Owner %s not found, already deleted", ownerPath)
		} else {

			ownerUserBytes, err := ioutil.ReadFile(ownerPath)
			if err != nil {
				outErr = err
				outStatus = http.StatusInternalServerError
			}

			ownerUser := string(ownerUserBytes)
			if ownerUser != api.User {
				outErr = errors.New("User not authorized to delete")
				outStatus = http.StatusForbidden
			}
		}

		err = os.Remove(ownPath)
		if err != nil {
			logger.Printf("Failed to delete %s: %s", ownPath, err)
		} else {
			logger.Printf("Deleted resource %s", ownPath)
		}

		err = os.Remove(ownerPath)
		if err != nil {
			logger.Printf("Failed to delete %s: %s", ownerPath, err)
		} else {
			logger.Printf("Deleted owner %s", ownerPath)
		}
	})

	if outErr != nil {
		return outErr, outStatus
	}

	w.WriteHeader(http.StatusNoContent)
	return nil, http.StatusNoContent
}

func apiHandler(w http.ResponseWriter, r *http.Request, logger *log.Logger) (error, int) {

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 3 {
		return fmt.Errorf("Bad path: %s", r.URL.Path), http.StatusBadRequest
	}

	format := parts[1]
	id := parts[2]

	if !reUUID.MatchString(id) {
		return fmt.Errorf("Not an UUID: %s", id), http.StatusBadRequest
	}

	identity, err := authenticate(r)
	if err != nil {
		return err, http.StatusUnauthorized
	}

	logger.Printf("Authorized as: %s\n", identity)

	var extension string

	switch format {
	case "videos":
		extension = ".mp4"
		break
	case "thubmnails":
		extension = ".jpg"
		break
	default:
		return fmt.Errorf("Unsupported format: %s", format), http.StatusBadRequest
	}

	api := apiData{
		ID:        id,
		User:      identity,
		Format:    format,
		Extension: extension,
	}

	switch r.Method {
	case "PUT":
		return uploadHandler(w, r, &api, logger)
	case "DELETE":
		return deleteHandler(w, r, &api, logger)
	default:
		return fmt.Errorf("Unsupported method: %s", r.Method), http.StatusMethodNotAllowed
	}
}

func httpHandler(w http.ResponseWriter, r *http.Request) {
	buffer := new(bytes.Buffer)
	reqID := atomic.AddInt32(&requestID, 1)
	reqIDString := fmt.Sprintf("r-%d: ", int(reqID))
	logger := log.New(buffer, reqIDString, log.LstdFlags)

	startTime := time.Now().UTC()
	err, status := apiHandler(w, r, logger)
	endTime := time.Now().UTC()
	duration := endTime.Sub(startTime)

	if err != nil {
		logger.Printf("Fail %d (%.2fs): %s\n", status, duration.Seconds(), err.Error())
		http.Error(w, err.Error(), status)
	} else {
		logger.Printf("Success %d (%.2fs)", status, duration.Seconds())
	}

	log.Printf("Output for request %d\n%s\n", reqID, buffer.String())
}

func main() {
	layersApiUri = strings.TrimSuffix(os.Getenv("LAYERS_API_URI"), "/")
	downloadBase = os.Getenv("ACHMINUP_DOWNLOAD_PATH")
	srcBase = path.Join(os.Getenv("ACHMINUP_PROCESS_PATH"), "src")
	dstBase = path.Join(os.Getenv("ACHMINUP_PROCESS_PATH"), "dst")
	serveBase = os.Getenv("ACHMINUP_SERVE_PATH")

	paths := []string{
		downloadBase, srcBase, dstBase, serveBase,
	}
	for _, root := range paths {
		os.MkdirAll(path.Join(root, "videos"), 0777)
		os.MkdirAll(path.Join(root, "thumbnails"), 0777)
	}

	http.HandleFunc("/", httpHandler)
	http.ListenAndServe(":8080", nil)
}
