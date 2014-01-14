FROM            ubuntu:12.04
MAINTAINER      James Sharp <james@ortootech.com>

ENTRYPOINT ["/opt/hipache-etcd/hipache-etcd"]

ADD        ./bin/linux_amd64/hipache-etcd /opt/hipache-etcd/hipache-etcd