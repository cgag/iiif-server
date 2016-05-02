#!/bin/sh

go build
sudo docker build -t iiif-server .
sudo docker run -p 8080:8080 iiif-server
