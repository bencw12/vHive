/*
 * Copyright 2015 The gRPC Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
import io.grpc.Grpc;
import io.grpc.InsecureServerCredentials;
import io.grpc.Server;
import io.grpc.stub.StreamObserver;
import java.io.IOException;
import java.io.InputStream;
import java.util.concurrent.TimeUnit;
import java.util.logging.Logger;
import io.grpc.examples.helloworld.*;
import okio.Buffer;
import java.util.Random;

import java.awt.image.BufferedImage;

import javax.imageio.ImageIO;

import java.io.File;
/**
 * Server that manages startup/shutdown of a {@code Greeter} server.
 */
public class Main extends GreeterGrpc.GreeterImplBase {
    private static final Logger logger = Logger.getLogger(HelloWorldServer.class.getName());

    private Server server;
    private int[][] a;
    private int[][] b;

    public Main() {
        int n = 100;
        // setup
        Random rand = new Random();

        a = new int[n][n];
        b = new int[n][n];
        for (int i = 0; i < n; i += 1) {
            for (int j = 0; j < n; j += 1) {
                a[i][j] = rand.nextInt(10);
                b[i][j] = rand.nextInt(10);
            }
        }
    }


    private static void exp_round(int[][] src, int[][] dst, int n) {
        for (int i = 0; i < n; i += 1) {
            for (int j = 0; j < n; j += 1) {
                int res_i_j = 0;
                for (int k = 0; k < n; k += 1) {
                    res_i_j += src[i][k] * src[k][j];
                }
                dst[i][j] = res_i_j;
            }
        }
    }

    public void start() throws IOException {
        int port = 50051;
        server = Grpc.newServerBuilderForPort(port, InsecureServerCredentials.create())
            .addService(this)
            .build()
            .start();
        Runtime.getRuntime().addShutdownHook(new Thread() {
                @Override
                public void run() {
                    try {
                        Main.this.stop();
                    } catch (InterruptedException e) {
                        e.printStackTrace(System.err);
                    }
                }
            });
    }

    private void stop() throws InterruptedException {
        if (server != null) {
            server.shutdown().awaitTermination(30, TimeUnit.SECONDS);
        }
    }

    /**
     * Await termination on the main thread since the grpc library uses daemon threads.
     */
    private void blockUntilShutdown() throws InterruptedException {
        if (server != null) {
            server.awaitTermination();
        }
    }

    public void snapshotPrepare() {
        for (int i = 0; i < 3; i++) {
            System.gc();
            Runtime.getRuntime().gc();
        }
    }
    class TestException extends Exception
    {
        // Parameterless Constructor
        public TestException() {}

        // Constructor that accepts a message
        public TestException(String message)
        {
            super(message);
        }
    }

    @Override
    public void sayHello(HelloRequest req, StreamObserver<HelloReply> responseObserver) {

        if (req.getName().equals("record")) {
            this.snapshotPrepare();
        }
        long start = System.nanoTime();
        exp_round(a, b, 100);
        long end = System.nanoTime();
        System.out.println("iteration time ns = " + (end - start));

        String msg = "";
        if (req.getName().equals("replay")) {
            msg = String.valueOf(end - start);
        }
        HelloReply reply = HelloReply.newBuilder().setMessage("Hello, " + req.getName() + "_response!").build();
        responseObserver.onNext(reply);
        responseObserver.onCompleted();

        // try {
        //     if (end > 0) {
        //         throw new TestException(String.valueOf(end - start));
        //     }
        // } catch (TestException e) {
        //     System.out.println(e.getMessage());
        // }
    }
    /**
     * Main launches the server from the command line.
     */
    public static void main(String[] args) throws IOException, InterruptedException {
        final Main server = new Main();
        server.start();
        server.blockUntilShutdown();
    }
}
