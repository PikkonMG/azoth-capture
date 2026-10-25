package video

import "strconv"

// The Windows recorder is ffmpeg with its gfxcapture source (Windows Graphics
// Capture, ffmpeg 8.1 and newer). This file builds its command lines; it has
// no build tag so the tests run on every system.

const (
	// gameExeMatch finds the game window by its program name, so the player
	// never picks a window. gfxcapture takes an ECMAScript regular expression.
	gameExeMatch = `(?i)^NewWorld\.exe$`
	wgcFPS       = 60
	// Clips are scaled to fit 1920x1080, like on Linux.
	wgcMaxW, wgcMaxH = 1920, 1080
	progressPeriod   = "0.5" // seconds between -progress reports
)

// encoder is one H.264 encoder and its settings for good-quality clips.
type encoder struct {
	name string
	args []string
}

// encoders are tried in this order: the GPU encoders first (NVIDIA, AMD,
// Intel, then Windows' Media Foundation), the CPU encoder last.
var encoders = []encoder{
	{"h264_nvenc", []string{"-preset", "p4", "-rc", "vbr", "-cq", "23"}},
	{"h264_amf", []string{"-quality", "balanced", "-rc", "cqp", "-qp_i", "23", "-qp_p", "23"}},
	{"h264_qsv", []string{"-preset", "medium", "-global_quality", "23"}},
	{"h264_mf", []string{"-b:v", "12M"}},
	{"libx264", []string{"-preset", "veryfast", "-crf", "23"}},
}

// encoderTestArgs encode a few frames of a test picture with enc, to learn
// whether this PC can use it.
func encoderTestArgs(enc encoder) []string {
	a := []string{"-hide_banner", "-loglevel", "error", "-nostdin",
		"-f", "lavfi", "-i", "color=c=black:s=256x144:r=30:d=0.2", "-pix_fmt", "yuv420p", "-c:v", enc.name}
	a = append(a, enc.args...)
	return append(a, "-f", "null", "-")
}

// gfxcaptureSource is the filter graph that captures the game window. Its
// frames live on the GPU; hwdownload brings them back for the encoder.
func gfxcaptureSource() string {
	return "gfxcapture=window_exe=" + gameExeMatch + ":max_framerate=" + strconv.Itoa(wgcFPS) +
		",hwdownload,format=bgra"
}

// recordArgs is the ffmpeg command line for one clip from source (the
// gfxcapture graph; tests pass a test picture). The encoder writes to out.
// ffmpeg stops cleanly when it reads "q" on its input.
func recordArgs(source string, enc encoder, out string) []string {
	scale := "scale=w='min(" + strconv.Itoa(wgcMaxW) + ",iw)':h='min(" + strconv.Itoa(wgcMaxH) +
		",ih)':force_original_aspect_ratio=decrease:force_divisible_by=2,format=yuv420p"
	a := []string{"-hide_banner", "-loglevel", "error", "-y",
		"-stats_period", progressPeriod, "-progress", "pipe:1",
		"-f", "lavfi", "-i", source, "-vf", scale, "-r", strconv.Itoa(wgcFPS), "-c:v", enc.name}
	a = append(a, enc.args...)
	return append(a, out)
}
