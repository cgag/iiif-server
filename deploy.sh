#!/bin/bash
set -euo pipefail

go build
sudo docker build -t iiif-server .
sudo docker save iiif-server > iiif-server.tar
pigz -9 -c iiif-server.tar > iiif-server.tar.gz

rsync -avz --progress iiif-server.service iiif-server.tar.gz core@trickster:

ssh core@trickster '
  sudo mv iiif-server.service /etc/systemd/system/iiif-server.service
  sudo systemctl daemon-reload
  sudo docker load < iiif-server.tar.gz
  sudo systemctl restart iiif-server
'

