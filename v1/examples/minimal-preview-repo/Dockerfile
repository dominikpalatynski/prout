FROM busybox:1.36.1

WORKDIR /site

COPY index.html /site/index.html

EXPOSE 8080

CMD ["httpd", "-f", "-p", "8080", "-h", "/site"]
