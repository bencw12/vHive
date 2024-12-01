FROM amazoncorretto:21-alpine3.20

COPY Main.java /
RUN mkdir /jar
COPY ./lib/* /jar/
RUN mkdir -p /io/grpc/examples/helloworld
COPY ./io/grpc/examples/helloworld/* /io/grpc/examples/helloworld/
EXPOSE 50051
STOPSIGNAL SIGKILL
WORKDIR /
RUN javac -cp "/:/jar/*" Main.java

CMD ["java", "-cp", "/:/jar/*:", "Main"]


