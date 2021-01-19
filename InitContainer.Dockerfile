FROM alpine:latest
RUN apk --no-cache add curl
RUN curl -L "https://repository.sonatype.org/service/local/artifact/maven/redirect?r=central-proxy&g=com.rookout&a=rook&v=LATEST" -o rook.jar
RUN mkdir /var/rookout
CMD ["cp", "rook.jar", "/var/rookout/rook.jar"]