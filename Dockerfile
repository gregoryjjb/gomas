# syntax=docker/dockerfile:1

FROM golang:1.19.4-bullseye

RUN apt-get update && apt-get install -y \
    curl bzip2 \
    gcc-arm-linux-gnueabihf \
    gcc-aarch64-linux-gnu

ENV ALSA_LIB_VERSION 1.2.7

RUN mkdir /alsa && \
    curl "ftp://ftp.alsa-project.org/pub/lib/alsa-lib-${ALSA_LIB_VERSION}.tar.bz2" -o /alsa/alsa-lib-${ALSA_LIB_VERSION}.tar.bz2

# https://www.programering.com/a/MTN0UDMwATk.html
# https://stackoverflow.com/questions/36195926/alsa-util-1-1-0-arm-cross-compile-issue
# RUN cd /alsa && \
#     tar -xvf alsa-lib-${ALSA_LIB_VERSION}.tar.bz2 && \
#     cd alsa-lib-${ALSA_LIB_VERSION} && \
    

# ALSA for aarch64 (64-bit ARM)
RUN cd /alsa && \
    tar -xvf alsa-lib-${ALSA_LIB_VERSION}.tar.bz2 && \
    cd alsa-lib-${ALSA_LIB_VERSION} && \
    CC=aarch64-linux-gnu-gcc ./configure --host=arm-linux && \
    make && \
    make install

# ALSA for arm (32-bit ARM)
# RUN cd /alsa && \
#     tar -xvf alsa-lib-${ALSA_LIB_VERSION}.tar.bz2 && \
#     cd alsa-lib-${ALSA_LIB_VERSION} && \
#     CC=arm-linux-gnueabihf-gcc-10 ./configure --host=arm-linux && \
#     make && \
#     make install

WORKDIR /gomas
