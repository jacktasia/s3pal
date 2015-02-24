package main

import (
	"strconv"
)

func getUploadForm(config S3palConfig) string {

	uploadEndpoint := "http://" + config.Server.Host + ":" + strconv.Itoa(config.Server.Port) + "/upload/file"

	return `<html>
 <title>s3pal uploader to ` + config.Aws.Bucket + `</title>
 <style type="text/css">
	.box {
		float:left;width:450px;height:450px;float:left;border:1px solid black;margin:5px;padding:5px;
	}
 </style>
 <body>
	<h1>s3pal</h1>
	<p>This needs to look better.</p>
	<div class="box">
		<h2>Upload to ` + config.Aws.Bucket + `</h2>


		<form action="` + uploadEndpoint + `/upload/file" method="post" enctype="multipart/form-data" id="upload-form">

			Prefix: <input type="text" name="prefix" value="` + config.Server.Prefix + `" style="width:200px">
			<br>
			<p id="msg">Drag/Drop file</p>
			<input type="file" name="file" id="file" style="width:200px;height:200px;border:1px dashed #ccc;;">
			<br><br>
			<input type="button" id="upload" value="Upload">
			<br>
		</form>
	</div>

	<div class="box">
		<h2>Result</h2>
		<div id="result"></div>
	</div>

	<script>
		var uploadForm = document.getElementById("upload-form");

		var doUpload = function() {
			uploadForm.style.display = 'none';
			document.getElementById("msg").innerHTML = 'Uploading...';

			var uploadData = new FormData(uploadForm);
			var xhr = new XMLHttpRequest();

			xhr.onreadystatechange = function(e) {
				if (xhr.readyState === 4) {
					document.getElementById("msg").innerHTML = 'Done.';
					var json = JSON.parse(xhr.responseText);
					var a = document.createElement("a");
					a.href = json.url;
					a.textContent = json.url;
					document.getElementById('result').appendChild(a);
				}
			}
			xhr.open("POST", "` + uploadEndpoint + `", true);

			console.log(uploadData);
			xhr.send(uploadData);
		}

		uploadForm.addEventListener("change", function(e) {
			doUpload();
		});

		document.getElementById('upload').addEventListener("click", function(e) {
			doUpload();
		});

	</script>
 </body>
 </html>
`
}
