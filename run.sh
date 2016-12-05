#!/bin/bash
set -euo pipefail

rm iiif-server
go build
sudo docker build -t iiif-server .
sudo docker run -p 7777:8080 iiif-server
