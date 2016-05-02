FROM frolvlad/alpine-glibc

ADD ./iiif-server /iiif-server

EXPOSE 8080
CMD /iiif-server
