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

import java.awt.*;
import java.awt.image.BufferedImage;

import javax.imageio.ImageIO;

import java.io.File;
/**
 * Server that manages startup/shutdown of a {@code Greeter} server.
 */
public class Main {
    private static final Logger logger = Logger.getLogger(HelloWorldServer.class.getName());

    private Server server;

    private void start() throws IOException {
        /* The port on which the server should run */
        int port = 50051;
        server = Grpc.newServerBuilderForPort(port, InsecureServerCredentials.create())
            .addService(new GreeterImpl())
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

    
    public static void snapshotPrepare() {
        for (int i = 0; i < 3; i++) {
            System.gc();
            Runtime.getRuntime().gc();
        }
    }

    /**
     * Main launches the server from the command line.
     */
    public static void main(String[] args) throws IOException, InterruptedException {
        // long t_start = System.nanoTime();
        // int n = 100;
        // Random rand = new Random();

        // int[][] a = new int[n][n];
        // int[][] b = new int[n][n];
        // for (int i = 0; i < n; i += 1) {
        //     for (int j = 0; j < n; j += 1) {
        //         a[i][j] = rand.nextInt(10);
        //         b[i][j] = i == j ? 1 : 0;
        //     }
        // }

        // long t_setup = System.nanoTime();
        // System.out.println("setup done in " + ((t_setup - t_start) / 1000) + "us");

        // t_start = System.nanoTime();
        // Main.mul(a, b, n);
        // long t_end = System.nanoTime();
        // System.out.println("mul done in " + ((t_end - t_start) / 1000) + "us");

        final Main server = new Main();
        server.start();
        server.blockUntilShutdown();
    }

    // return time
    private static long mul(int[][] src, int[][] dst, int n) {
        long start = System.nanoTime();
        for (int i = 0; i < n; i += 1) {
            for (int j = 0; j < n; j += 1) {
                int res_i_j = 0;
                for (int k = 0; k < n; k += 1) {
                    res_i_j += src[i][k] * src[k][j];
                }
                dst[i][j] = res_i_j;
            }
        }
        return System.nanoTime() - start;
    }

    public static void handle(int[][] a, int[][] b, int size) {
        // use a as dst
        mul(a, b, size);
    }

    static class GreeterImpl extends GreeterGrpc.GreeterImplBase {
        private static final int SIZE = 100;
        private static String minioAddress = System.getenv("MINIO_ADDRESS");
        public int[][] a;
        public int[][] b;

        public GreeterImpl() {
            Random rand = new Random();

            this.a = new int[SIZE][SIZE];
            this.b = new int[SIZE][SIZE];
            for (int i = 0; i < SIZE; i += 1) {
                for (int j = 0; j < SIZE; j += 1) {
                    this.a[i][j] = rand.nextInt(10);
                    this.b[i][j] = i == j ? 1 : 0;
                }
            }
        }
	
        @Override
        public void sayHello(HelloRequest req, StreamObserver<HelloReply> responseObserver) {

            if (req.getName().equals("record")) {
                for (int i = 0; i < 50; i++) {
                    Main.handle(a, b, SIZE);
                }
            } else {
                Main.handle(a, b, SIZE);
            }

            HelloReply reply = HelloReply.newBuilder().setMessage("Hello, " + req.getName() + "_response!").build();
            responseObserver.onNext(reply);
            responseObserver.onCompleted();
        }
    }
}
