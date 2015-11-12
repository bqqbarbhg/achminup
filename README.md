Basic Video Uploader
====================

Upload video and thumbnail data for the server (Apache, Nginx etc. ) to serve as static content.

Usage
-----

- `POST /upload.php?type=video&id=3f747dfd-a8c1-47c2-ba37-8ebc720de045` with raw mp4 data in the body to upload a video
- `POST /upload.php?type=thumbnail&id=3f747dfd-a8c1-47c2-ba37-8ebc720de045` with raw jpg data in the body to upload a thumbnail

Both return the url of the uploaded object as plain text.

Setup
-----

Copy `sample_secrets.php` and rename it to `secrets.php` and change the `$base_url` value to the preferred root path to the uploaded
videos or thumbnails.

