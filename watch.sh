#!/usr/bin/env bash

# Set direcctory to the current working directory
cd "${0%/*}"

./transcode_watcher.py /var/achminup-uploads/temp/videos_to_transcode /var/achminup-uploads/temp/transcode_work /var/achminup-uploads/videos
