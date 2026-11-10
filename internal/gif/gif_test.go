package gif

import (
	"bytes"
	stdgif "image/gif"
	"reflect"
	"testing"

	"lazascrub/internal/meta"
	"lazascrub/internal/samples"
)

func TestScanFindsEveryKind(t *testing.T) {
	found, err := Scan(samples.GIF())
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := meta.Lookup(found, "Comment", "Comment"); !ok || f.Value != samples.Comment {
		t.Errorf("comment = %q", f.Value)
	}
	if f, ok := meta.Lookup(found, "XMP", "dc:creator"); !ok || f.Value != samples.Author {
		t.Errorf("XMP creator = %q", f.Value)
	}
	if _, ok := meta.Lookup(found, "After the content", "Hidden data"); !ok {
		t.Error("data after the picture was not reported")
	}
}

func TestCleanRemovesMetadataAndKeepsTheAnimation(t *testing.T) {
	original := samples.GIF()
	cleaned, err := Clean(original)
	if err != nil {
		t.Fatal(err)
	}
	before, err := stdgif.DecodeAll(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	after, err := stdgif.DecodeAll(bytes.NewReader(cleaned))
	if err != nil {
		t.Fatalf("the animation no longer decodes: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Error("the cleaned animation differs from the original")
	}
	if after.LoopCount != 0 || len(after.Image) != 2 {
		t.Error("looping or frames were lost")
	}
	for _, secret := range samples.Secrets() {
		if bytes.Contains(cleaned, []byte(secret)) {
			t.Errorf("%q survived cleaning", secret)
		}
	}
	if left, _ := Scan(cleaned); len(left) != 0 {
		t.Errorf("%d fields are still in the cleaned file: %v", len(left), left)
	}
	again, _ := Clean(cleaned)
	if !bytes.Equal(again, cleaned) {
		t.Error("cleaning a clean file changed it")
	}
}

func TestDamagedFilesDoNotCrash(t *testing.T) {
	whole := samples.GIF()
	for cut := 0; cut < len(whole); cut += 3 {
		Scan(whole[:cut])
		Clean(whole[:cut])
	}
}
