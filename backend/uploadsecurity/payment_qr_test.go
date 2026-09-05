package uploadsecurity

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func encodedTestImage(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			canvas.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	var output bytes.Buffer
	var err error
	if format == "jpeg" {
		err = jpeg.Encode(&output, canvas, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&output, canvas)
	}
	if err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestSanitizePaymentQRCodeDecodesAndReencodesSupportedRasterImages(t *testing.T) {
	for _, format := range []string{"png", "jpeg"} {
		t.Run(format, func(t *testing.T) {
			cleaned, err := SanitizePaymentQRCode(bytes.NewReader(encodedTestImage(t, format, 128, 128)))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(cleaned.png, []byte("\x89PNG\r\n\x1a\n")) {
				t.Fatal("sanitized image was not re-encoded as PNG")
			}
			if _, decodedFormat, err := image.Decode(bytes.NewReader(cleaned.png)); err != nil || decodedFormat != "png" {
				t.Fatalf("sanitized image = %q, %v", decodedFormat, err)
			}
		})
	}
}

func TestSanitizePaymentQRCodeAcceptsRealWebPAndReencodesIt(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "new", "public", "images", "room-logos", "crown-shield.webp"))
	if err != nil {
		t.Fatal(err)
	}
	cleaned, err := SanitizePaymentQRCode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(cleaned.png, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("WebP was not normalized to PNG")
	}
}

func TestSanitizePaymentQRCodeRejectsSVGTrailingPayloadAndUnsafeDimensions(t *testing.T) {
	valid := encodedTestImage(t, "png", 128, 128)
	validJPEG := encodedTestImage(t, "jpeg", 128, 128)
	tests := []struct {
		name string
		data []byte
	}{
		{name: "svg script", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)},
		{name: "png with script appended", data: append(append([]byte(nil), valid...), []byte(`<script>alert(1)</script>`)...)},
		{name: "png with javascript payload", data: append(append([]byte(nil), valid...), []byte(`javascript:alert(1)`)...)},
		{name: "jpeg with payload hidden before a second end marker", data: append(append(append([]byte(nil), validJPEG...), []byte("unrelated-payload")...), 0xff, 0xd9)},
		{name: "too small", data: encodedTestImage(t, "png", 32, 32)},
		{name: "too many pixels", data: encodedTestImage(t, "png", 4096, 1025)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := SanitizePaymentQRCode(bytes.NewReader(test.data)); err == nil {
				t.Fatal("unsafe QR code was accepted")
			}
		})
	}
}

func TestPaymentQRCodeStorageUsesOwnerScopeRandomNameAndPrivateFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BACKEND_UPLOAD_DIR", root)
	cleaned, err := SanitizePaymentQRCode(bytes.NewReader(encodedTestImage(t, "png", 128, 128)))
	if err != nil {
		t.Fatal(err)
	}
	filename, err := StorePaymentQRCode(37, 91, cleaned)
	if err != nil {
		t.Fatal(err)
	}
	if !paymentQRCodeFilenamePattern.MatchString(filename) || strings.Contains(filename, "..") {
		t.Fatalf("unsafe generated filename %q", filename)
	}
	target := filepath.Join(root, ".private", "member-payment-qr", "37", "91", filename)
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("QR code mode = %o, want 600", info.Mode().Perm())
	}
	stream, streamSize, err := OpenPaymentQRCode(37, 91, filename)
	if err != nil {
		t.Fatal("open owner QR stream:", err)
	}
	if streamSize != int64(len(cleaned.png)) {
		_ = stream.Close()
		t.Fatalf("QR stream size = %d, want %d", streamSize, len(cleaned.png))
	}
	var streamedSignature [8]byte
	if _, err := stream.Read(streamedSignature[:]); err != nil {
		_ = stream.Close()
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(streamedSignature[:], []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("streamed QR signature = %x", streamedSignature)
	}
	if _, _, err := OpenPaymentQRCode(37, 92, filename); !errorsIsNotExist(err) {
		t.Fatalf("cross-user read error = %v, want not-exist", err)
	}
	if _, _, err := OpenPaymentQRCode(38, 91, filename); !errorsIsNotExist(err) {
		t.Fatalf("cross-workspace read error = %v, want not-exist", err)
	}
	if _, _, err := OpenPaymentQRCode(37, 91, "../../"+filename); err == nil {
		t.Fatal("path traversal filename was accepted")
	}
	symlinkName := strings.Repeat("a", 32) + ".png"
	if err := os.Symlink(target, filepath.Join(filepath.Dir(target), symlinkName)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenPaymentQRCode(37, 91, symlinkName); err == nil {
		t.Fatal("symlink QR code was accepted")
	}
	if err := RemovePaymentQRCode(37, 91, symlinkName); err == nil {
		t.Fatal("symlink QR code cleanup was accepted")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("symlink cleanup touched its target: %v", err)
	}
	if err := os.WriteFile(target, []byte(`<script>alert(1)</script>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenPaymentQRCode(37, 91, filename); err == nil {
		t.Fatal("tampered stored QR code was accepted")
	}
	if err := RemovePaymentQRCode(37, 91, filename); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("removed QR code still exists: %v", err)
	}
}

func TestPaymentQRCodeStorageRejectsSymlinkOrRootUploadDirectory(t *testing.T) {
	cleaned, err := SanitizePaymentQRCode(bytes.NewReader(encodedTestImage(t, "png", 128, 128)))
	if err != nil {
		t.Fatal(err)
	}
	t.Run("symlink", func(t *testing.T) {
		base := t.TempDir()
		realRoot := filepath.Join(base, "real")
		if err := os.Mkdir(realRoot, 0o750); err != nil {
			t.Fatal(err)
		}
		linkedRoot := filepath.Join(base, "linked")
		if err := os.Symlink(realRoot, linkedRoot); err != nil {
			t.Fatal(err)
		}
		t.Setenv("BACKEND_UPLOAD_DIR", linkedRoot)
		if _, err := StorePaymentQRCode(37, 91, cleaned); err == nil {
			t.Fatal("symlink upload root was accepted")
		}
	})
	t.Run("filesystem root", func(t *testing.T) {
		t.Setenv("BACKEND_UPLOAD_DIR", string(filepath.Separator))
		if _, err := StorePaymentQRCode(37, 91, cleaned); err == nil {
			t.Fatal("filesystem root was accepted as an upload directory")
		}
	})
}

func TestPaymentQRCodeStorageConcurrentNamesRemainUniqueAndOwnerScoped(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BACKEND_UPLOAD_DIR", root)
	cleaned, err := SanitizePaymentQRCode(bytes.NewReader(encodedTestImage(t, "png", 128, 128)))
	if err != nil {
		t.Fatal(err)
	}

	const workers = 24
	names := make(chan string, workers)
	errors := make(chan error, workers)
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			name, storeErr := StorePaymentQRCode(37, 91, cleaned)
			if storeErr != nil {
				errors <- storeErr
				return
			}
			names <- name
		}()
	}
	group.Wait()
	close(names)
	close(errors)
	for storeErr := range errors {
		t.Fatal(storeErr)
	}
	unique := make(map[string]struct{}, workers)
	for name := range names {
		if !paymentQRCodeFilenamePattern.MatchString(name) {
			t.Fatalf("concurrent store generated unsafe name %q", name)
		}
		if _, exists := unique[name]; exists {
			t.Fatalf("concurrent store reused name %q", name)
		}
		unique[name] = struct{}{}
		if escaped, _, readErr := OpenPaymentQRCode(37, 92, name); !errorsIsNotExist(readErr) {
			if escaped != nil {
				_ = escaped.Close()
			}
			t.Fatalf("concurrent QR escaped owner scope: %v", readErr)
		}
	}
	if len(unique) != workers {
		t.Fatalf("stored %d unique QR codes, want %d", len(unique), workers)
	}
}

func TestRemovePaymentQRCodeIsIdempotentAfterInterruptedAcknowledgement(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BACKEND_UPLOAD_DIR", root)
	cleaned, err := SanitizePaymentQRCode(bytes.NewReader(encodedTestImage(t, "png", 128, 128)))
	if err != nil {
		t.Fatal(err)
	}
	filename, err := StorePaymentQRCode(37, 91, cleaned)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, ".private", "member-payment-qr", "37", "91", filename)
	// Equivalent to a process dying after unlink but before clearing the
	// durable database queue reference.
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := RemovePaymentQRCode(37, 91, filename); err != nil {
		t.Fatalf("retry after interrupted acknowledgement was not idempotent: %v", err)
	}
}

func TestListPaymentQRCodeFilesReturnsOnlyCanonicalOwnerScopedFiles(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BACKEND_UPLOAD_DIR", root)
	cleaned, err := SanitizePaymentQRCode(bytes.NewReader(encodedTestImage(t, "png", 128, 128)))
	if err != nil {
		t.Fatal(err)
	}
	second, err := StorePaymentQRCode(38, 92, cleaned)
	if err != nil {
		t.Fatal(err)
	}
	first, err := StorePaymentQRCode(37, 91, cleaned)
	if err != nil {
		t.Fatal(err)
	}
	crashFilename := strings.Repeat("c", 32) + ".png"
	crashDirectory := filepath.Join(root, ".private", "member-payment-qr", "39", "93")
	if err := os.MkdirAll(crashDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crashDirectory, crashFilename), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	files, err := ListPaymentQRCodeFiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []PaymentQRCodeFile{
		{WorkspaceID: 37, UserID: 91, Filename: first},
		{WorkspaceID: 38, UserID: 92, Filename: second},
		{WorkspaceID: 39, UserID: 93, Filename: crashFilename},
	}
	if len(files) != len(want) {
		t.Fatalf("listed files = %#v, want %#v", files, want)
	}
	for index := range want {
		if files[index] != want[index] {
			t.Fatalf("listed file %d = %#v, want %#v", index, files[index], want[index])
		}
	}
}

func TestListPaymentQRCodeFilesFailsClosedOnUnexpectedTreeEntries(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, qrRoot string)
	}{
		{
			name: "non canonical workspace",
			setup: func(t *testing.T, qrRoot string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(qrRoot, "01"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unexpected nesting",
			setup: func(t *testing.T, qrRoot string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(qrRoot, "37", "91", "extra"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "non server filename",
			setup: func(t *testing.T, qrRoot string) {
				t.Helper()
				directory := filepath.Join(qrRoot, "37", "91")
				if err := os.MkdirAll(directory, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, "customer.png"), []byte("not server owned"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlink",
			setup: func(t *testing.T, qrRoot string) {
				t.Helper()
				directory := filepath.Join(qrRoot, "37", "91")
				if err := os.MkdirAll(directory, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), filepath.Join(directory, strings.Repeat("a", 32)+".png")); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("BACKEND_UPLOAD_DIR", root)
			qrRoot := filepath.Join(root, ".private", "member-payment-qr")
			if err := os.MkdirAll(qrRoot, 0o700); err != nil {
				t.Fatal(err)
			}
			test.setup(t, qrRoot)
			if _, err := ListPaymentQRCodeFiles(context.Background()); err == nil {
				t.Fatal("unsafe private QR tree was accepted")
			}
		})
	}
}

func TestListPaymentQRCodeFilesTreatsMissingPrivateTreeAsEmpty(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BACKEND_UPLOAD_DIR", root)
	files, err := ListPaymentQRCodeFiles(context.Background())
	if err != nil || len(files) != 0 {
		t.Fatalf("missing QR tree = %#v, %v; want empty", files, err)
	}
	if _, err := ListPaymentQRCodeFiles(nil); err == nil {
		t.Fatal("QR listing accepted an unbounded nil context")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ListPaymentQRCodeFiles(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled QR listing error = %v, want context.Canceled", err)
	}
}

func errorsIsNotExist(err error) bool { return errors.Is(err, os.ErrNotExist) }
