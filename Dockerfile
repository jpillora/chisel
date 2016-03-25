FROM alpine
MAINTAINER dev@jpillora.com

#configure go path
ENV GOPATH /root/go
ENV PATH $PATH:/usr/local/go/bin:$GOPATH/bin

#package
ENV PACKAGE github.com/jpillora/chisel

#install go and deps, then package,
#move build binaries out then wipe build tools
RUN apk update && \
        apk add git go gzip && \
        go get -v $PACKAGE && \
        mv $GOPATH/bin/* /usr/local/bin/ && \
        rm -rf $GOPATH && \
        apk del git go gzip && \
        echo "Installed $PACKAGE"

#alternatively, git clone into $GOPATH/src,
#then go get -u $PACKAGE to update deps

#run package
ENTRYPOINT ["chisel"]
