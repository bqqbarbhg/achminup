package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
)

var downloadBase = "test/download"
var destinationBase = "test/dest"
var serveBase = "test/serve"
var reUUID = regexp.MustCompile("[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}")

func authenticate(r *http.Request) (identity string, err error) {
	url := os.Getenv("LAYERS_API_URI") + "/o/oauth2/userinfo"

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

func uploadHandler(w http.ResponseWriter, r *http.Request, api *apiData) {

	ownerSrcPath := path.Join(downloadBase, api.Format, api.ID+".owner.txt")
	ownerDstPath := path.Join(destinationBase, api.Format, api.ID+".owner.txt")
	ownerFile, err := os.Create(ownerSrcPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ownerFile.WriteString(api.User)
	err = ownerFile.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	outSrcPath := path.Join(downloadBase, api.Format, api.ID+api.Extension)
	outDstPath := path.Join(destinationBase, api.Format, api.ID+api.Extension)
	outFile, err := os.Create(outSrcPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_, err = io.Copy(outFile, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = outFile.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = os.Rename(ownerSrcPath, ownerDstPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = os.Rename(outSrcPath, outDstPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	url := os.Getenv("LAYERS_API_URI") + path.Join("achminup", api.Format, api.ID+api.Extension)
	fmt.Fprintf(w, "%s", url)
}

func deleteHandler(w http.ResponseWriter, r *http.Request, api *apiData) {
	ownerPath := path.Join(serveBase, api.Format, api.ID+".owner.txt")
	ownPath := path.Join(serveBase, api.Format, api.ID+api.Extension)

	ownerUserBytes, err := ioutil.ReadFile(ownerPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ownerUser := string(ownerUserBytes)
	if ownerUser != api.User {
		http.Error(w, "User not authorized to delete", http.StatusUnauthorized)
		return
	}

	err = os.Remove(ownPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = os.Remove(ownerPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Bad URL", http.StatusBadRequest)
		return
	}

	format := parts[2]
	id := parts[3]

	if !reUUID.MatchString(id) {
		http.Error(w, "Bad UUID", http.StatusBadRequest)
		return
	}

	identity, err := authenticate(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var extension string

	switch format {
	case "videos":
		extension = ".mp4"
		break
	case "thubmnails":
		extension = ".jpg"
		break
	default:
		http.Error(w, "unsupported format", http.StatusBadRequest)
		return
	}

	api := apiData{
		ID:        id,
		User:      identity,
		Format:    format,
		Extension: extension,
	}

	switch r.Method {
	case "PUT":
		uploadHandler(w, r, &api)
		return
	case "DELETE":
		deleteHandler(w, r, &api)
		return
	default:
		http.Error(w, "Unsupported method", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func main() {
	http.HandleFunc("/api/", apiHandler)
	http.ListenAndServe(":8080", nil)
}
