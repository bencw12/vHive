FROM amazoncorretto:21-alpine3.20

COPY Main.java /
RUN mkdir /jar
COPY ./lib/* /jar/
RUN mkdir -p /io/grpc/examples/helloworld
COPY ./io/grpc/examples/helloworld/* /io/grpc/examples/helloworld/
WORKDIR /
RUN javac -cp "/:/jar/*" Main.java

EXPOSE 50051
STOPSIGNAL SIGKILL
CMD ["java", "-cp", "/:/jar/*:", "Main"]


