FROM golang:1.13-buster

# Copy Source Code into Docker Image
ADD . /src
WORKDIR /src 

# Build oe-sgx-plugin
RUN go mod init sgxlkl-device-plugin
RUN go mod vendor
RUN go build

FROM ubuntu:18.04

COPY --from=0 /src/sgxlkl-device-plugin /usr/local/bin/sgxlkl-device-plugin
