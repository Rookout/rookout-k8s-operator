FROM registry.access.redhat.com/ubi8-minimal:8.5-230
RUN microdnf install curl
RUN curl -L "https://repository.sonatype.org/service/local/artifact/maven/redirect?r=central-proxy&g=com.rookout&a=rook&v=LATEST" -o rook.jar
CMD ["cp", "rook.jar", "/rookout/rook.jar"]