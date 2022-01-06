package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"io"
	"log"
	"os"

	ffmpeg "github.com/u2takey/ffmpeg-go"
	"gocv.io/x/gocv"
)

func getVideoSize(fileName string) (int, int) {
	log.Println("Getting video size for", fileName)
	data, err := ffmpeg.Probe(fileName)
	if err != nil {
		panic(err)
	}
	log.Println("got video info", data)
	type VideoInfo struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			Width     int
			Height    int
		} `json:"streams"`
	}
	vInfo := &VideoInfo{}
	err = json.Unmarshal([]byte(data), vInfo)
	if err != nil {
		panic(err)
	}
	for _, s := range vInfo.Streams {
		if s.CodecType == "video" {
			return s.Width, s.Height
		}
	}
	return 0, 0
}

func startFFmpegProcess1(infileName string, writer io.WriteCloser) <-chan error {
	log.Println("Starting ffmpeg process1")
	done := make(chan error)
	go func() {
		err := ffmpeg.Input(infileName).
			Output("pipe:",
				ffmpeg.KwArgs{
					"format": "rawvideo", "pix_fmt": "bgr24",
				}).
			WithOutput(writer).
			Run()
		log.Println("ffmpeg process1 done")
		_ = writer.Close()
		done <- err
		close(done)
	}()
	return done
}

func process(reader io.ReadCloser, writer io.WriteCloser, w, h int) {
	go func() {
		frameSize := w * h * 3
		buf := make([]byte, frameSize, frameSize)
		for {
			n, err := io.ReadFull(reader, buf)
			if n == 0 || err == io.EOF {
				_ = writer.Close()
				return
			} else if n != frameSize || err != nil {
				panic(fmt.Sprintf("read error: %d, %s", n, err))
			}

			////if this open, the video picture will be gray
			// for i := range buf {
			// 	buf[i] = buf[i] / 3
			// }

			n, err = writer.Write(buf)
			if n != frameSize || err != nil {
				panic(fmt.Sprintf("write error: %d, %s", n, err))
			}
		}
	}()
	return
}

func startFFmpegProcess2(outfileName string, buf io.Reader, width, height int) <-chan error {
	log.Println("Starting ffmpeg process2")
	done := make(chan error)
	go func() {
		err := ffmpeg.Input("pipe:",
			ffmpeg.KwArgs{"format": "rawvideo",
				"pix_fmt": "bgr24", "s": fmt.Sprintf("%dx%d", width, height),
			}).
			Output(outfileName, ffmpeg.KwArgs{
				"pix_fmt": "yuv420p", "c:v": "libx264", "preset": "ultrafast", "f": "flv", "r": "25",
			}).
			OverWriteOutput().
			WithInput(buf).
			Run()
		log.Println("ffmpeg process2 done")
		done <- err
		close(done)
	}()
	return done
}

func runExampleStream(inFile, outFile string) {
	w, h := getVideoSize(inFile)
	log.Println(w, h)

	pr1, pw1 := io.Pipe()
	pr2, pw2 := io.Pipe()
	done1 := startFFmpegProcess1(inFile, pw1)
	process(pr1, pw2, w, h)
	done2 := startFFmpegProcess2(outFile, pr2, w, h)
	err := <-done1
	if err != nil {
		panic(err)
	}
	err = <-done2
	if err != nil {
		panic(err)
	}
	log.Println("Done")
}

func runOpenCvFaceDetectWithCamera(input, output string) {
	xmlFile := "../examples/sample_data/haarcascade_frontalface_default.xml"

	webcam, err := gocv.OpenVideoCapture(input)
	if err != nil {
		fmt.Printf("error opening video capture device: %v\n", input)
		return
	}
	defer webcam.Close()

	// prepare image matrix
	img := gocv.NewMat()
	defer img.Close()

	if ok := webcam.Read(&img); !ok {
		panic(fmt.Sprintf("Cannot read device %v", input))
	}
	fmt.Printf("img: %vX%v\n", img.Cols(), img.Rows())

	pr1, pw1 := io.Pipe()
	// writeProcess("./sample_data/face_detect.mp4", pr1, img.Cols(), img.Rows())
	// writeProcess(output, pr1, img.Cols(), img.Rows())
	startFFmpegProcess2(output, pr1, img.Cols(), img.Rows())

	// color for the rect when faces detected
	blue := color.RGBA{B: 255}

	// load classifier to recognize faces
	classifier := gocv.NewCascadeClassifier()
	defer classifier.Close()

	if !classifier.Load(xmlFile) {
		fmt.Printf("Error reading cascade file: %v\n", xmlFile)
		return
	}

	fmt.Printf("Start reading device: %v\n", input)
	for i := 0; i < 2000; i++ {
		if ok := webcam.Read(&img); !ok {
			fmt.Printf("Device closed: %v\n", input)
			return
		}
		if img.Empty() {
			continue
		}

		// detect faces
		rects := classifier.DetectMultiScale(img)
		fmt.Printf("found %d faces\n", len(rects))

		// draw a rectangle around each face on the original image, along with text identifing as "Human"
		for _, r := range rects {
			gocv.Rectangle(&img, r, blue, 3)

			size := gocv.GetTextSize("Human", gocv.FontHersheyPlain, 1.2, 2)
			pt := image.Pt(r.Min.X+(r.Min.X/2)-(size.X/2), r.Min.Y-2)
			gocv.PutText(&img, "Human", pt, gocv.FontHersheyPlain, 1.2, blue, 2)
		}
		// pw1.Write(img.ToBytes())

		//test buf to image
		img1, err := gocv.NewMatFromBytes(img.Rows(), img.Cols(), gocv.MatTypeCV8UC3, img.ToBytes())
		if err != nil {
			fmt.Println("change fail")
		}
		gocv.IMWrite("test1.jpg", img)
		gocv.IMWrite("test.jpg", img1)
		os.Exit(1)
	}
	pw1.Close()
	log.Println("Done")
}

func main() {
	input := "rtsp://admin:cvdev2018@192.168.1.51"
	output := "rtmp://127.0.0.1:1935/live/stream"
	fmt.Println("test rtmp begin")
	// runExampleStream(input, output)
	runOpenCvFaceDetectWithCamera(input, output)
	fmt.Println("test rtmp end")
}
