package interactive

import "testing"

func TestDescribeElapsedKeepsActualTimeUnits(t *testing.T) {
	for seconds, want := range map[int64]string{0: "", 1: "1秒", 60: "1分钟", 14 * 60: "14分钟", 15 * 60: "1刻", 30 * 60: "2刻", 3600: "1小时", 7200: "1时辰", 3 * 3600: "3小时", 6 * 3600: "3时辰", 24 * 3600: "1日"} {
		if got := DescribeElapsed(seconds); got != want {
			t.Errorf("%d seconds: got %q want %q", seconds, got, want)
		}
	}
}
