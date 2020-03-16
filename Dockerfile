# Now copy it into our base image.
FROM golang:1.12
LABEL maintainer="chaitanya_bhatt@intuit.com"
RUN mkdir /app
# copy everything in the root directory into our /app directory
ADD . /app
# execute further commands inside our /app directory
WORKDIR /app

RUN tar -xvzf test-data/qbo-1.log.tar --directory test-data/.

RUN go build -o loadgen .

CMD ["/app/loader"]

