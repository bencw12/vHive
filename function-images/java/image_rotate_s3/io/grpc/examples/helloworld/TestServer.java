package io.grpc.examples.helloworld;

import io.grpc.Grpc;
import io.grpc.InsecureServerCredentials;
import io.grpc.Server;
import io.grpc.ServerBuilder;
import io.grpc.stub.StreamObserver;
import java.io.IOException;

public class TestServer {
    public static void main(String[] args) throws IOException, InterruptedException {
        // Create the gRPC server
        // Server server = ServerBuilder.forPort(50051)
        //         .addService(new GreeterImpl())
        //         .build();

	HelloReply reply = null;

        // Start the server
        // server.start();
        // System.out.println("Server started, listening on 50051");

        // // Add shutdown hook to stop the server gracefully
        // Runtime.getRuntime().addShutdownHook(new Thread(() -> {
        //     System.out.println("Shutting down gRPC server");
        //     server.shutdown();
        // }));

        // // Block until the server shuts down
        // server.awaitTermination();
    }

    // Implement the Greeter service
    static class GreeterImpl extends GreeterGrpc.GreeterImplBase {
        @Override
        public void sayHello(HelloRequest req, StreamObserver<HelloReply> responseObserver) {
            // Generate the response
            HelloReply response = HelloReply.newBuilder()
                    .setMessage("Hello, " + req.getName())
                    .build();

            // Send the response
            responseObserver.onNext(response);
            responseObserver.onCompleted();
        }
    }
}
