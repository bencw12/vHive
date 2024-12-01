import javax.imageio.ImageIO;

import java.awt.*;
import java.awt.image.BufferedImage;

import java.io.BufferedReader;
import java.io.File;
import java.io.FileOutputStream;
import java.io.FileReader;
import java.io.IOException;
import java.io.PrintWriter;

import com.sun.jna.Library;
import com.sun.jna.Native;

import io.minio.MinioClient;
import io.minio.GetObjectArgs;

class Resizer {
    private BufferedImage image;

    public Resizer(BufferedImage image) {
	this.image = image;
    }
    
    public BufferedImage rotate(double angle) {
	int width = this.image.getWidth();
	int height = this.image.getHeight();
	int newWidth = (int) Math.abs(width * Math.cos(angle) + height * Math.sin(angle));
        int newHeight = (int) Math.abs(width * Math.sin(angle) + height * Math.cos(angle));

	BufferedImage rotatedImage = new BufferedImage(newWidth, newHeight, this.image.getType());
        Graphics2D g2d = rotatedImage.createGraphics();

        g2d.setRenderingHint(RenderingHints.KEY_INTERPOLATION, RenderingHints.VALUE_INTERPOLATION_BILINEAR);

        g2d.translate((newWidth - width) / 2, (newHeight - height) / 2);
        g2d.rotate(angle, width / 2.0, height / 2.0);
        g2d.drawImage(this.image, 0, 0, null);

	g2d.dispose();

        return rotatedImage;
    }

    public static void main(String args[]) {
	MinioClient client = MinioClient.builder()
	    .endpoint("http://localhost:9000")
	    .credentials("minioadmin", "minioadmin")
	    .build();

	BufferedImage image;
	BufferedImage rotated;
	Resizer resizer;
	double angle = Math.toRadians(90);
    }
}
