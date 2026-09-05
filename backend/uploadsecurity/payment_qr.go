package uploadsecurity

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	_ "golang.org/x/image/webp"
)

const (
	// Uploaded QR codes are deliberately much smaller than general activity
	// artwork. Both the compressed input and decoded pixel count are bounded so
	// a tiny decompression bomb cannot consume unbounded memory.
	MaxPaymentQRCodeBytes       int64 = 4 << 20
	maxPaymentQRCodeOutputBytes int64 = 12 << 20
	maxPaymentQRCodeDimension         = 4096
	minPaymentQRCodeDimension         = 64
	maxPaymentQRCodePixels      int64 = 4_194_304
)

var paymentQRCodeFilenamePattern = regexp.MustCompile(`\A[0-9a-f]{32}\.png\z`)

// PaymentQRCode is an opaque, decoded and metadata-free PNG. Callers cannot
// construct one from untrusted bytes without passing through the sanitizer.
type PaymentQRCode struct {
	png []byte
}

// PaymentQRCodeFile is a server-generated file discovered below the private
// owner-scoped QR root. ListPaymentQRCodeFiles is the only supported way to
// enumerate that tree; callers never receive an arbitrary filesystem path.
type PaymentQRCodeFile struct {
	WorkspaceID uint64
	UserID      uint64
	Filename    string
}

// SanitizePaymentQRCode validates the actual container and decoded image,
// ignores the client filename and content type, then emits a fresh PNG. The
// fresh encoding strips metadata, scripts and polyglot payloads.
func SanitizePaymentQRCode(source io.Reader) (*PaymentQRCode, error) {
	if source == nil {
		return nil, errors.New("请选择二维码图片")
	}
	raw, err := io.ReadAll(io.LimitReader(source, MaxPaymentQRCodeBytes+1))
	if err != nil {
		return nil, errors.New("二维码图片读取失败")
	}
	if len(raw) == 0 {
		return nil, errors.New("二维码图片不能为空")
	}
	if int64(len(raw)) > MaxPaymentQRCodeBytes {
		return nil, errors.New("二维码图片不能超过 4MB")
	}
	lower := bytes.ToLower(raw)
	if bytes.Contains(lower, []byte("<script")) || bytes.Contains(lower, []byte("<svg")) || bytes.Contains(lower, []byte("javascript:")) {
		return nil, errors.New("二维码图片包含脚本或 SVG 载荷")
	}

	wantFormat, err := exactPaymentQRCodeContainer(raw)
	if err != nil {
		return nil, err
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || format != wantFormat {
		return nil, errors.New("二维码不是有效的 PNG、JPG 或 WebP 图片")
	}
	if config.Width < minPaymentQRCodeDimension || config.Height < minPaymentQRCodeDimension ||
		config.Width > maxPaymentQRCodeDimension || config.Height > maxPaymentQRCodeDimension ||
		int64(config.Width)*int64(config.Height) > maxPaymentQRCodePixels {
		return nil, errors.New("二维码像素尺寸需在 64×64 至 4096×4096 以内，且总像素不超过 419 万")
	}

	decoded, decodedFormat, err := image.Decode(bytes.NewReader(raw))
	if err != nil || decodedFormat != wantFormat {
		return nil, errors.New("二维码图片内容已损坏或格式不正确")
	}
	var cleaned boundedBuffer
	cleaned.limit = maxPaymentQRCodeOutputBytes
	if err := png.Encode(&cleaned, decoded); err != nil {
		if errors.Is(err, errSanitizedImageTooLarge) {
			return nil, errors.New("二维码图片重新编码后过大")
		}
		return nil, errors.New("二维码图片安全处理失败")
	}
	return &PaymentQRCode{png: append([]byte(nil), cleaned.Bytes()...)}, nil
}

// exactPaymentQRCodeContainer recognizes bytes rather than extensions or MIME
// headers and rejects data appended after the end of the image container.
func exactPaymentQRCodeContainer(raw []byte) (string, error) {
	switch {
	case bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")):
		if !exactPNGContainer(raw) {
			return "", errors.New("PNG 二维码包含损坏区块或多余载荷")
		}
		return "png", nil
	case len(raw) >= 4 && raw[0] == 0xff && raw[1] == 0xd8:
		if !exactJPEGContainer(raw) {
			return "", errors.New("JPG 二维码包含损坏内容或多余载荷")
		}
		return "jpeg", nil
	case len(raw) >= 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP":
		declared := uint64(binary.LittleEndian.Uint32(raw[4:8])) + 8
		if declared != uint64(len(raw)) {
			return "", errors.New("WebP 二维码包含损坏内容或多余载荷")
		}
		return "webp", nil
	default:
		return "", errors.New("二维码仅支持真实的 PNG、JPG 或 WebP 图片，不支持 SVG")
	}
}

// exactJPEGContainer walks marker segments and entropy-coded scans through the
// first EOI marker. JPEG decoders are allowed to ignore bytes after EOI; doing
// that here would let a second EOI hide an appended payload from our simple
// end-of-file check.
func exactJPEGContainer(raw []byte) bool {
	if len(raw) < 4 || raw[0] != 0xff || raw[1] != 0xd8 {
		return false
	}
	offset := 2
	inScan := false
	sawScan := false
	for offset < len(raw) {
		if raw[offset] != 0xff {
			if !inScan {
				return false
			}
			offset++
			continue
		}
		// Marker fill bytes are permitted before the marker code.
		for offset < len(raw) && raw[offset] == 0xff {
			offset++
		}
		if offset >= len(raw) {
			return false
		}
		marker := raw[offset]
		offset++
		if inScan {
			switch {
			case marker == 0x00: // entropy byte 0xff, escaped as 0xff 0x00
				continue
			case marker >= 0xd0 && marker <= 0xd7: // restart marker
				continue
			default:
				inScan = false
			}
		} else if marker == 0x00 {
			return false
		}

		switch {
		case marker == 0xd9: // EOI
			return sawScan && offset == len(raw)
		case marker == 0xd8 || marker == 0x01 || marker >= 0xd0 && marker <= 0xd7:
			// A second SOI, TEM, or restart marker outside entropy data is not
			// part of the bounded image container accepted by this endpoint.
			return false
		default:
			if offset+2 > len(raw) {
				return false
			}
			length := int(binary.BigEndian.Uint16(raw[offset : offset+2]))
			if length < 2 || offset+length > len(raw) {
				return false
			}
			offset += length
			if marker == 0xda { // SOS
				sawScan = true
				inScan = true
			}
		}
	}
	return false
}

func exactPNGContainer(raw []byte) bool {
	if len(raw) < 20 {
		return false
	}
	offset := 8
	for offset <= len(raw)-12 {
		length := uint64(binary.BigEndian.Uint32(raw[offset : offset+4]))
		end := uint64(offset) + 12 + length
		if end > uint64(len(raw)) {
			return false
		}
		chunkType := string(raw[offset+4 : offset+8])
		offset = int(end)
		if chunkType == "IEND" {
			return length == 0 && offset == len(raw)
		}
	}
	return false
}

var errSanitizedImageTooLarge = errors.New("sanitized image too large")

type boundedBuffer struct {
	bytes.Buffer
	limit int64
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	remaining := buffer.limit - int64(buffer.Len())
	if remaining <= 0 || int64(len(data)) > remaining {
		return 0, errSanitizedImageTooLarge
	}
	return buffer.Buffer.Write(data)
}

func uploadRoot() string {
	if root := strings.TrimSpace(os.Getenv("BACKEND_UPLOAD_DIR")); root != "" {
		return root
	}
	return "uploads"
}

func paymentQRCodeDirectory(workspaceID, userID uint64) string {
	return filepath.Join(filepath.Clean(uploadRoot()), ".private", "member-payment-qr", strconv.FormatUint(workspaceID, 10), strconv.FormatUint(userID, 10))
}

// StorePaymentQRCode uses a cryptographically random server-side filename.
// No part of an uploaded filename or content type reaches the filesystem.
func StorePaymentQRCode(workspaceID, userID uint64, image *PaymentQRCode) (string, error) {
	if image == nil || len(image.png) == 0 {
		return "", errors.New("sanitized QR code is empty")
	}
	directory := paymentQRCodeDirectory(workspaceID, userID)
	if err := ensurePrivateDirectory(directory); err != nil {
		return "", err
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate QR code filename: %w", err)
	}
	filename := hex.EncodeToString(random) + ".png"
	target := filepath.Join(directory, filename)
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create QR code file: %w", err)
	}
	if _, err := file.Write(image.png); err != nil {
		_ = file.Close()
		_ = os.Remove(target)
		return "", fmt.Errorf("write QR code file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(target)
		return "", fmt.Errorf("close QR code file: %w", err)
	}
	return filename, nil
}

func ensurePrivateDirectory(directory string) error {
	root := filepath.Clean(uploadRoot())
	if root == string(filepath.Separator) || root == "." {
		return errors.New("unsafe upload root")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return fmt.Errorf("create upload root: %w", err)
	}
	if err := requireRealDirectory(root); err != nil {
		return err
	}
	privateRoot := filepath.Join(root, ".private")
	parts := []string{
		privateRoot,
		filepath.Join(privateRoot, "member-payment-qr"),
		filepath.Dir(directory),
		directory,
	}
	for _, part := range parts {
		if err := os.Mkdir(part, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create private upload directory: %w", err)
		}
		if err := requireRealDirectory(part); err != nil {
			return err
		}
		if err := os.Chmod(part, 0o700); err != nil {
			return fmt.Errorf("secure private upload directory: %w", err)
		}
	}
	return nil
}

func validatePrivateDirectory(directory string) error {
	root := filepath.Clean(uploadRoot())
	if root == string(filepath.Separator) || root == "." {
		return errors.New("unsafe upload root")
	}
	for _, part := range []string{
		root,
		filepath.Join(root, ".private"),
		filepath.Join(root, ".private", "member-payment-qr"),
		filepath.Dir(directory),
		directory,
	} {
		if err := requireRealDirectory(part); err != nil {
			return err
		}
	}
	return nil
}

func requireRealDirectory(directory string) error {
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("inspect private upload directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("private upload path is not a real directory")
	}
	return nil
}

// OpenPaymentQRCode reconstructs the owner-scoped path and returns only an
// already-open, inode-verified regular file. Callers can stream the bounded
// file without first allocating its full contents in memory.
func OpenPaymentQRCode(workspaceID, userID uint64, filename string) (*os.File, int64, error) {
	if !paymentQRCodeFilenamePattern.MatchString(filename) {
		return nil, 0, errors.New("invalid QR code filename")
	}
	directory := paymentQRCodeDirectory(workspaceID, userID)
	if err := validatePrivateDirectory(directory); err != nil {
		return nil, 0, err
	}
	target := filepath.Join(directory, filename)
	info, err := os.Lstat(target)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > maxPaymentQRCodeOutputBytes {
		return nil, 0, errors.New("invalid QR code file")
	}
	file, err := os.Open(target)
	if err != nil {
		return nil, 0, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, err
	}
	// Read from the already-open handle and require it to be the same inode
	// inspected above. A path swapped to a symlink between Lstat and Open cannot
	// redirect the authenticated response to another file.
	if !openedInfo.Mode().IsRegular() || openedInfo.Size() != info.Size() || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, 0, errors.New("QR code file changed during read")
	}
	if err := validateStoredPaymentQRCodePNG(file, openedInfo.Size()); err != nil {
		_ = file.Close()
		return nil, 0, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, 0, err
	}
	return file, openedInfo.Size(), nil
}

// validateStoredPaymentQRCodePNG verifies the complete PNG framing and CRCs
// with constant memory. It scans the already-open bounded handle once, then
// OpenPaymentQRCode rewinds it for HTTP streaming.
func validateStoredPaymentQRCodePNG(file *os.File, size int64) error {
	if file == nil || size < 20 || size > maxPaymentQRCodeOutputBytes {
		return errors.New("invalid QR code file")
	}
	var signature [8]byte
	if _, err := io.ReadFull(file, signature[:]); err != nil || !bytes.Equal(signature[:], []byte("\x89PNG\r\n\x1a\n")) {
		return errors.New("stored QR code is not a PNG")
	}
	offset := int64(len(signature))
	var header [8]byte
	var encodedCRC [4]byte
	for offset <= size-12 {
		if _, err := io.ReadFull(file, header[:]); err != nil {
			return errors.New("stored QR code has a truncated PNG chunk")
		}
		length := int64(binary.BigEndian.Uint32(header[:4]))
		if length > size-offset-12 {
			return errors.New("stored QR code has an invalid PNG chunk length")
		}
		digest := crc32.NewIEEE()
		_, _ = digest.Write(header[4:8])
		if _, err := io.CopyN(digest, file, length); err != nil {
			return errors.New("stored QR code has a truncated PNG chunk")
		}
		if _, err := io.ReadFull(file, encodedCRC[:]); err != nil || binary.BigEndian.Uint32(encodedCRC[:]) != digest.Sum32() {
			return errors.New("stored QR code has an invalid PNG checksum")
		}
		offset += 12 + length
		if string(header[4:8]) == "IEND" {
			if length != 0 || offset != size {
				return errors.New("stored QR code has trailing PNG payload")
			}
			return nil
		}
	}
	return errors.New("stored QR code is missing PNG end marker")
}

func RemovePaymentQRCode(workspaceID, userID uint64, filename string) error {
	if !paymentQRCodeFilenamePattern.MatchString(filename) {
		return errors.New("invalid QR code filename")
	}
	directory := paymentQRCodeDirectory(workspaceID, userID)
	if err := validatePrivateDirectory(directory); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	target := filepath.Join(directory, filename)
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("QR code cleanup target is not a regular file")
	}
	err = os.Remove(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ListPaymentQRCodeFiles walks only the exact server-owned
// workspace/user/random-name layout. It fails closed on symlinks, special
// files, unexpected nesting or names instead of silently skipping something a
// later cleanup could mistake for an orphan.
func ListPaymentQRCodeFiles(ctx context.Context) ([]PaymentQRCodeFile, error) {
	if ctx == nil {
		return nil, errors.New("payment QR listing requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root := filepath.Clean(uploadRoot())
	if root == string(filepath.Separator) || root == "." {
		return nil, errors.New("unsafe upload root")
	}
	if err := requireRealDirectory(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []PaymentQRCodeFile{}, nil
		}
		return nil, err
	}
	privateRoot := filepath.Join(root, ".private")
	if err := requireRealDirectory(privateRoot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []PaymentQRCodeFile{}, nil
		}
		return nil, err
	}
	qrRoot := filepath.Join(privateRoot, "member-payment-qr")
	if err := requireRealDirectory(qrRoot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []PaymentQRCodeFile{}, nil
		}
		return nil, err
	}

	files := make([]PaymentQRCodeFile, 0)
	err := filepath.WalkDir(qrRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path == qrRoot {
			return nil
		}
		relative, err := filepath.Rel(qrRoot, path)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
			strings.ContainsAny(relative, "\r\n") {
			return errors.New("invalid payment QR storage path")
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("payment QR storage contains symlink: %s", relative)
		}

		if info.IsDir() {
			if len(parts) < 1 || len(parts) > 2 {
				return fmt.Errorf("payment QR storage contains unexpected directory: %s", relative)
			}
			for _, part := range parts {
				if _, ok := parseCanonicalPaymentQRCodeOwnerID(part); !ok {
					return fmt.Errorf("payment QR storage contains invalid owner directory: %s", relative)
				}
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("payment QR storage contains special file: %s", relative)
		}
		if len(parts) != 3 || !paymentQRCodeFilenamePattern.MatchString(parts[2]) {
			return fmt.Errorf("payment QR storage contains non-server file: %s", relative)
		}
		workspaceID, workspaceOK := parseCanonicalPaymentQRCodeOwnerID(parts[0])
		userID, userOK := parseCanonicalPaymentQRCodeOwnerID(parts[1])
		// A crash immediately after O_EXCL creation can leave a zero-byte
		// canonical file. It is still safe to enumerate (and, if unreferenced,
		// remove); authenticated reads continue to reject empty files.
		if !workspaceOK || !userOK || info.Size() > maxPaymentQRCodeOutputBytes {
			return fmt.Errorf("payment QR storage contains invalid file: %s", relative)
		}
		files = append(files, PaymentQRCodeFile{WorkspaceID: workspaceID, UserID: userID, Filename: parts[2]})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(left, right int) bool {
		if files[left].WorkspaceID != files[right].WorkspaceID {
			return files[left].WorkspaceID < files[right].WorkspaceID
		}
		if files[left].UserID != files[right].UserID {
			return files[left].UserID < files[right].UserID
		}
		return files[left].Filename < files[right].Filename
	})
	return files, nil
}

func parseCanonicalPaymentQRCodeOwnerID(value string) (uint64, bool) {
	if value == "" || value[0] == '0' {
		return 0, false
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	return parsed, err == nil && parsed != 0 && strconv.FormatUint(parsed, 10) == value
}
