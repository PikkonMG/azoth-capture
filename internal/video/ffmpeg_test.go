package video

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestFFmpegPathPrefersOwnCopy(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, ffmpegFile())
	if err := os.WriteFile(own, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := FFmpegPath(dir); err != nil || got != own {
		t.Errorf("FFmpegPath = %q, %v; want the app's own copy", got, err)
	}
}

func TestVerifyAndExtract(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ff.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range map[string]string{"ffmpeg-n9/bin/ffmpeg.exe": "binary", "ffmpeg-n9/bin/ffprobe.exe": "other"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, body)
	}
	zw.Close()
	f.Close()

	b, _ := os.ReadFile(zipPath)
	sum := sha256.Sum256(b)
	if err := verifySHA256(zipPath, hex.EncodeToString(sum[:])); err != nil {
		t.Errorf("a matching download was rejected: %v", err)
	}
	if err := verifySHA256(zipPath, strings.Repeat("0", 64)); err == nil {
		t.Error("a changed download was accepted")
	}
	dst := filepath.Join(dir, "ffmpeg.exe")
	if err := extractFromZip(zipPath, "/bin/ffmpeg.exe", dst); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "binary" {
		t.Errorf("extracted %q", got)
	}
	if err := extractFromZip(zipPath, "/bin/missing.exe", dst); err == nil {
		t.Error("a missing file was extracted")
	}
}

func TestReadProgressReportsFirstFrameOnce(t *testing.T) {
	text := "frame=0\nout_time_us=0\nprogress=continue\n" +
		"frame=12\nout_time_us=200000\nprogress=continue\n" +
		"frame=40\nout_time_us=666000\nprogress=end\nsome log line\n"
	var calls []int64
	var logged []string
	readProgress(strings.NewReader(text), func(us int64) { calls = append(calls, us) }, func(l string) { logged = append(logged, l) })
	if !slices.Equal(calls, []int64{200000}) || !slices.Equal(logged, []string{"some log line"}) {
		t.Errorf("first-frame calls %v, log %v", calls, logged)
	}
}

func TestRecordArgs(t *testing.T) {
	a := recordArgs(gfxcaptureSource(), encoders[0], "/s/gameplay-001.mkv")
	src := a[slices.Index(a, "-i")+1]
	if !strings.HasPrefix(src, "gfxcapture=window_exe=(?i)^NewWorld\\.exe$:") || !strings.HasSuffix(src, ",hwdownload,format=bgra") {
		t.Errorf("source = %q", src)
	}
	if a[len(a)-1] != "/s/gameplay-001.mkv" || !slices.Contains(a, "h264_nvenc") || !slices.Contains(a, "pipe:1") {
		t.Errorf("args = %q", a)
	}
	if slices.ContainsFunc(a, func(s string) bool { return strings.Contains(s, "dshow") || strings.Contains(s, "wasapi") }) {
		t.Errorf("args = %q: Windows clips record no sound device", a)
	}
}

// A real recording with the same command line, from a test picture instead
// of the game window: ffmpeg reports its first frame and stops cleanly on "q".
func TestRecordStopsOnQ(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	libx264 := encoders[len(encoders)-1]
	if exec.Command(ffmpeg, encoderTestArgs(libx264)...).Run() != nil {
		t.Skip("this ffmpeg has no libx264")
	}
	out := filepath.Join(t.TempDir(), "clip.mkv")
	cmd := exec.Command(ffmpeg, recordArgs("testsrc=size=640x360:rate=60", libx264, out)...)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	first := make(chan int64, 1)
	go readProgress(stdout, func(us int64) { first <- us }, func(string) {})
	select {
	case <-first:
	case <-time.After(20 * time.Second):
		t.Fatal("no first frame reported")
	}
	io.WriteString(stdin, "q")
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("ffmpeg did not stop cleanly: %v", err)
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		t.Errorf("no clip written: %v", err)
	}
}
