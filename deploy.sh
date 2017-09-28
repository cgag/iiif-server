#!/bin/bash

./build-push.sh

cd /home/cgag/play/src/personal-server/packet/iiif-server || exit
scp ./iiif-server.service packet:/home/core
ssh packet "
  sudo mv iiif-server.service /etc/systemd/system
  sudo systemctl enable iiif-server.service
  sudo systemctl restart iiif-server.service
"
