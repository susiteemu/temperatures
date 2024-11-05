FROM amd64/alpine:latest

WORKDIR /server

COPY ["ruuvitag-httpserver", "./"]
COPY [".env", "./"]
COPY ["config.yml", "./"]

ENTRYPOINT ["./ruuvitag-httpserver"]
