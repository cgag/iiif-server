FROM debian:jessie

RUN apt-get update -y && apt-get install -y \
    wget              \
    build-essential   \
    libopenjpeg5      \
    libopenjpeg-dev   \
    libopenjp2-7-dev  \
    libgif-dev        \
    libbz2-dev        \
    libdjvulibre-dev  \
    fftw-dev          \
    libfontconfig-dev \
    libfreetype6-dev  \
    libjbig-dev       \
    liblcms2-dev      \
    liblqr-dev        \
    libltdl7-dev      \
    lzma-dev          \
    libopenexr-dev    \
    libpangocairo-1.0 \
    libpng-dev        \
    libtiff5-dev      \
    libwmf-dev        \
    libxml2-dev       \
    zlib1g-dev        \
    libmagick-dev

RUN wget http://www.imagemagick.org/download/ImageMagick.tar.gz  && \
    tar -xvzf ImageMagick.tar.gz && \
    cd ImageMagick-7.0.3-8 &&       \
    ./configure &&                  \
    make -j5 &&                     \
    make install &&                 \
    ldconfig

ADD iiif-server /iiif-server
ADD images /images

EXPOSE 8080
CMD /iiif-server
