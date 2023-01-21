FROM alpine:latest
COPY krmfnbuiltin /usr/local/bin/config-function
CMD ["config-function"]
