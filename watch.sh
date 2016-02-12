#!/usr/bin/env bash

# Set direcctory to the current working directory
cd "${0%/*}"

./transcode_watcher.py temp/videos_to_transcode temp/transcode_work /var/achminup-uploads/videos
