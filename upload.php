<?php

$id = $_GET["id"];
$type = $_GET["type"];

$uuid_r = '/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/';
if (!preg_match($uuid_r, $id)) {
        die("Not an UUID");
}

if ($type == "video") {
        $path = "videos/$id.mp4";
} else if ($type == "thumbnail") {
        $path = "thumbnails/$id.jpg";
} else {
        die("Unknown type");
}

$in = fopen("php://input", "r");
$out = fopen($path, "w");

stream_copy_to_stream($in, $out);

fclose($in);
fclose($out);

chmod($path, 0666);

$base_url = "http://".$_SERVER["HTTP_HOST"]."/achminup/"

echo $base_url.$path;

