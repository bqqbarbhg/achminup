<?php

require 'secrets.php';

$id = $_GET["id"];
$type = $_GET["type"];

$uuid_r = '/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/';
if (!preg_match($uuid_r, $id)) {
        die("Not an UUID");
}

if ($type == "video") {

        $dl_path = "temp/video_downloads/$id.mp4";
        $out_path = "temp/videos_to_transcode/$id.mp4";
        $url_path = "videos/$id.mp4";

} else if ($type == "thumbnail") {

        $dl_path = "temp/thumbnail_downloads/$id.jpg";
        $out_path = "/var/achimup-uploads/thumbnails/$id.jpg";
        $url_path = "thumbnails/$id.jpg";

} else {
        die("Unknown type");
}

$in = fopen("php://input", "r");
$out = fopen($dl_path, "w");

stream_copy_to_stream($in, $out);

fclose($in);
fclose($out);

chmod($dl_path, 0666);
rename($dl_path, $out_path);

echo $base_url.$url_path;

