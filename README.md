# s3pal

A server + cli S3 tool for uploading and listing files.

## Overview

You can run a simple server to handle uploads to s3 by running:

`./s3pal server`

that could handle uploads with HTML like this:

	 <form action="http://localhost:8080/upload/file" method="post" enctype="multipart/form-data">
		 <label for="file">Filename:</label>
		 <input type="file" name="file" id="file">
		 <input type="submit" name="submit" value="Submit">
	 </form>

You can upload a file on your computer

`./s3pal upload ~/Pictures/mycat.jpg`

or upload a file on the internet:

`./s3pal upload "https://www.google.com/images/srpr/logo11w.png"`


## Why? (Demo)

I made this mainly for frontend demos that need access to S3, but don't need any other backend. Here's [my first project using s3pal](http://jackangers.com/imgix-wall)

##Configuring

You configure `s3pal` using a toml config file. `s3pal` automatically looks for `s3pal.toml` in the working directory. Alternatively you can use the `--config` flag to provide a path.

#### Example config
	[aws]
	access_key = "AKI..."
	secret_key = "Iw3..."
	bucket = "mybucket"
	region = "us-west-2"

	[server]
	port = 8080


## Building

Assuming you have a proper go environment setup this should be as easy as:

    git clone git@github.com:jacktasia/s3pal.git
    cd s3pal
	go get ./... # install dependencies
	go build s3pal.go
	./s3pal help # test help
	#cp sample_s3pal.toml s3pal.toml
	#emacs s3pal.toml
	#./s3pal server

## Warnings

This is a very young project. I would not use it in serious production. That said, [here's a demo using it](http://jackangers.com/imgix-wall)