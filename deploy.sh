#!/bin/bash

./build-push.sh

# cd /home/cgag/play/src/personal-server/packet/iiif-server || exit
# scp ./iiif-server.service packet:/home/core
# ssh packet "
#   sudo mv iiif-server.service /etc/systemd/system
#   sudo systemctl enable iiif-server.service
#   sudo systemctl restart iiif-server.service
# "

# TODO(cgag): a problem if we change which cluster is default or whatever
# lmao, how is rolling restart still not a thing??
kubectl patch deployment iiif-server -p '{"spec":{"template":{"spec":{"containers":[{"name":"iiif-server","env":[{"name":"LAST_MANUAL_RESTART","value":"'$(date +%s)'"}]}]}}}}'
