FROM centos:7

RUN yum install -y git

ENTRYPOINT ["jx-verify"]

COPY ./build/linux/jx-verify /usr/bin/jx-verify