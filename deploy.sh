#!/bin/bash
set -euo pipefail

go build
docker build -t gcr.io/cgag-gke/iiif-server .
gcloud docker -- push gcr.io/cgag-gke/iiif-server


# sudo docker save iiif-server > iiif-server.tar
# pigz -9 -c iiif-server.tar > iiif-server.tar.gz

# rsync -avz --progress iiif-server.service core@trickster: &
# #rsync -avz --progress iiif-server.tar.gz core@trickster: &
# wait

# ssh core@trickster '
#   sudo mv iiif-server.service /etc/systemd/system/iiif-server.service
#   sudo systemctl daemon-reload
# #  sudo docker load < iiif-server.tar.gz
#   sudo systemctl restart iiif-server
# '
