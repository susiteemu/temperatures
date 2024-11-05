FROM amd64/alpine:latest

WORKDIR /server

COPY ["ruuvitag-httpserver", ".env", "config.yml", "./"]

ENTRYPOINT ["./ruuvitag-httpserver"]
