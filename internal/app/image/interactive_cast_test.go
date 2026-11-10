package imageapp

import (
	"strings"
	"testing"

	imagegen "ta/internal/image/generation"
)

// The authored bodies this build ships do not agree on the appearance heading.
// Two variants exist in the shipped world book, so both have to be read.
func TestAuthorAppearanceReadsBothHeadingVariants(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
		deny string
	}{
		{
			name: "形象与性格",
			body: "## 1. 身份与近况\n身份：家族出身弟子。\n\n## 2. 形象、性格与内在矛盾\n五官清秀，衣饰整洁。\n\n## 3. 出身经历与当前目标\n已在黄枫谷修行。\n",
			want: "五官清秀，衣饰整洁。",
			deny: "黄枫谷",
		},
		{
			name: "外貌与辨识",
			body: "## 1. 身份与近况\n身份：掌柜。\n\n## 2. 外貌与辨识\n圆脸，左手有旧疤。\n\n## 3. 性格\n谨慎。\n",
			want: "圆脸，左手有旧疤。",
			deny: "谨慎",
		},
		{
			name: "CRLF body",
			body: "## 1. 身份与近况\r\n身份：散修。\r\n\r\n## 2. 外貌与辨识\r\n瘦高，惯穿灰袍。\r\n\r\n## 3. 经历\r\n云游。\r\n",
			want: "瘦高，惯穿灰袍。",
			deny: "云游",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := authorAppearance(testCase.body)
			if !strings.Contains(got, testCase.want) {
				t.Fatalf("appearance missing %q: %q", testCase.want, got)
			}
			if strings.Contains(got, testCase.deny) {
				t.Fatalf("appearance leaked another section %q: %q", testCase.deny, got)
			}
		})
	}
}

// A body with no appearance section contributes no appearance. Returning the
// whole body would hand the image model the person's plot.
func TestAuthorAppearanceOmitsBodyWithoutAppearanceSection(t *testing.T) {
	if got := authorAppearance("## 1. 身份与近况\n身份：船工。\n"); got != "" {
		t.Fatalf("expected no appearance, got %q", got)
	}
}

func TestAuthorAppearanceBoundsOneSection(t *testing.T) {
	long := strings.Repeat("相貌端正。", 200)
	got := authorAppearance("## 2. 外貌与辨识\n" + long)
	if runes := []rune(got); len(runes) > 400 {
		t.Fatalf("appearance not bounded: %d runes", len(runes))
	}
}

// A project asset path is what the run may read. Anything that could reach
// outside the workspace is dropped rather than handed to a reader.
func TestProjectAssetPathRejectsEscape(t *testing.T) {
	for _, raw := range []string{"", "/etc/passwd", `\windows`, "assets/../../secret.png", "C:/secret.png", "https://example.com/a.png"} {
		if path, ok := projectAssetPath(raw); ok {
			t.Fatalf("%q accepted as %q", raw, path)
		}
	}
	if path, ok := projectAssetPath("assets/lore/images/hero/image.png"); !ok || path != "assets/lore/images/hero/image.png" {
		t.Fatalf("project-relative path rejected: %q %v", path, ok)
	}
}

// The cast travels as a string through config, so the encoding has to survive a
// round trip unchanged, including the picture locator.
func TestReferenceCastRoundTripsThroughConfig(t *testing.T) {
	cast := []imagegen.ReferencePerson{{
		Name:       "陈巧倩",
		SourceKey:  "canon_chen_qiaoqian",
		Appearance: "五官清秀，衣饰整洁。",
		Picture:    imagegen.ReferencePicture(imagegen.PictureShipped, "canon_chen_qiaoqian"),
	}}
	encoded := interactiveImageReferencePeople(cast)
	if encoded == "" {
		t.Fatal("empty encoding for a non-empty cast")
	}
	decoded := imagegen.DecodeReferencePeople(encoded)
	if len(decoded) != 1 || decoded[0] != cast[0] {
		t.Fatalf("cast changed in transit: %#v", decoded)
	}
	source, name := imagegen.SplitReferencePicture(decoded[0].Picture)
	if source != imagegen.PictureShipped || name != "canon_chen_qiaoqian" {
		t.Fatalf("locator did not split: %q %q", source, name)
	}
}

func TestReferenceCastEncodesEmptyAsEmptyString(t *testing.T) {
	if got := interactiveImageReferencePeople(nil); got != "" {
		t.Fatalf("empty cast encoded as %q", got)
	}
}
