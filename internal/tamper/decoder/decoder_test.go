package decoder

import (
	"strings"
	"testing"
)

// --- Base64Decoder Tests ---

func TestBase64Decoder(t *testing.T) {
	d := &Base64Decoder{}

	t.Run("name", func(t *testing.T) {
		if d.Name() != "base64" {
			t.Errorf("expected name 'base64', got %q", d.Name())
		}
	})

	t.Run("CanDecode valid base64", func(t *testing.T) {
		if !d.CanDecode("SGVsbG8=") {
			t.Error("expected to recognize valid base64")
		}
	})

	t.Run("CanDecode rejects odd length", func(t *testing.T) {
		if d.CanDecode("SGVsbG8") {
			t.Error("expected to reject odd-length string")
		}
	})

	t.Run("CanDecode rejects empty", func(t *testing.T) {
		if d.CanDecode("") {
			t.Error("expected to reject empty string")
		}
	})

	t.Run("CanDecode rejects non-base64 chars", func(t *testing.T) {
		if d.CanDecode("hello!") {
			t.Error("expected to reject non-base64 characters")
		}
	})

	t.Run("Decode valid base64", func(t *testing.T) {
		result, err := d.Decode("SGVsbG8gV29ybGQ=")
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if result != "Hello World" {
			t.Errorf("expected 'Hello World', got %q", result)
		}
	})

	t.Run("Decode invalid base64", func(t *testing.T) {
		_, err := d.Decode("AAAA====")
		if err == nil {
			t.Error("expected error for invalid base64")
		}
	})
}

// --- HexDecoder Tests ---

func TestHexDecoder(t *testing.T) {
	d := &HexDecoder{}

	t.Run("name", func(t *testing.T) {
		if d.Name() != "hex" {
			t.Errorf("expected name 'hex', got %q", d.Name())
		}
	})

	t.Run("CanDecode valid hex", func(t *testing.T) {
		if !d.CanDecode("48656c6c6f") {
			t.Error("expected to recognize valid hex")
		}
	})

	t.Run("CanDecode rejects odd length", func(t *testing.T) {
		if d.CanDecode("486") {
			t.Error("expected to reject odd-length hex")
		}
	})

	t.Run("CanDecode rejects invalid chars", func(t *testing.T) {
		if d.CanDecode("486G") {
			t.Error("expected to reject non-hex characters")
		}
	})

	t.Run("Decode valid hex", func(t *testing.T) {
		result, err := d.Decode("48656c6c6f")
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if result != "Hello" {
			t.Errorf("expected 'Hello', got %q", result)
		}
	})

	t.Run("Decode invalid hex", func(t *testing.T) {
		_, err := d.Decode("GG")
		if err == nil {
			t.Error("expected error for invalid hex")
		}
	})
}

// --- UnicodeDecoder Tests ---

func TestUnicodeDecoder(t *testing.T) {
	d := &UnicodeDecoder{}

	t.Run("name", func(t *testing.T) {
		if d.Name() != "unicode" {
			t.Errorf("expected name 'unicode', got %q", d.Name())
		}
	})

	t.Run("CanDetect", func(t *testing.T) {
		if !d.CanDecode(`\u0048\u0065\u006c\u006c\u006f`) {
			t.Error("expected to recognize unicode escapes")
		}
	})

	t.Run("CanDetect rejects plain text", func(t *testing.T) {
		if d.CanDecode("Hello") {
			t.Error("expected to reject plain text")
		}
	})

	t.Run("Decode unicode", func(t *testing.T) {
		result, err := d.Decode(`\u0048\u0065\u006c\u006c\u006f`)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if result != "Hello" {
			t.Errorf("expected 'Hello', got %q", result)
		}
	})

	t.Run("Decode preserves invalid sequences", func(t *testing.T) {
		result, err := d.Decode(`\uZZZZ`)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		// Should return original since \uZZZZ is not matched by the pattern
		if result != `\uZZZZ` {
			t.Errorf("expected original, got %q", result)
		}
	})
}

// --- URLDecoder Tests ---

func TestURLDecoder(t *testing.T) {
	d := &URLDecoder{}

	t.Run("name", func(t *testing.T) {
		if d.Name() != "url" {
			t.Errorf("expected name 'url', got %q", d.Name())
		}
	})

	t.Run("CanDetect", func(t *testing.T) {
		if !d.CanDecode("Hello%20World") {
			t.Error("expected to recognize URL encoding")
		}
	})

	t.Run("CanDetect rejects plain text", func(t *testing.T) {
		if d.CanDecode("Hello World") {
			t.Error("expected to reject plain text")
		}
	})

	t.Run("Decode URL encoded", func(t *testing.T) {
		result, err := d.Decode("Hello%20World")
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if result != "Hello World" {
			t.Errorf("expected 'Hello World', got %q", result)
		}
	})

	t.Run("Decode preserves invalid sequences", func(t *testing.T) {
		result, err := d.Decode("%ZZ")
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if !strings.Contains(result, "%ZZ") {
			t.Errorf("expected to preserve invalid sequence, got %q", result)
		}
	})
}

// --- HTMLDecoder Tests ---

func TestHTMLDecoder(t *testing.T) {
	d := &HTMLDecoder{}

	t.Run("name", func(t *testing.T) {
		if d.Name() != "html" {
			t.Errorf("expected name 'html', got %q", d.Name())
		}
	})

	t.Run("CanDetect named entities", func(t *testing.T) {
		if !d.CanDecode("&amp;lt;") {
			t.Error("expected to recognize HTML entities")
		}
	})

	t.Run("CanDetect rejects plain text", func(t *testing.T) {
		if d.CanDecode("Hello World") {
			t.Error("expected to reject plain text")
		}
	})

	t.Run("Decode named entities", func(t *testing.T) {
		result, err := d.Decode("&lt;script&gt;&amp;quot;test&amp;quot;&lt;/script&gt;")
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		// &amp; → &, then &quot; → " (both happen in single pass since &amp; is replaced first)
		if result != `<script>"test"</script>` {
			t.Errorf("expected decoded entities, got %q", result)
		}
	})

	t.Run("Decode numeric entities", func(t *testing.T) {
		result, err := d.Decode("&#72;&#101;&#108;&#108;&#111;")
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if result != "Hello" {
			t.Errorf("expected 'Hello', got %q", result)
		}
	})

	t.Run("Decode invalid numeric entity preserved", func(t *testing.T) {
		result, err := d.Decode("&#99999999999;")
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		// Very large number overflows uint32, should be preserved
		if result != "&#99999999999;" {
			t.Errorf("expected preserved entity, got %q", result)
		}
	})
}

// --- DecoderManager Tests ---

func TestDecoderManager(t *testing.T) {
	t.Run("new manager has default decoders", func(t *testing.T) {
		m := NewDecoderManager()
		if len(m.decoders) != 5 {
			t.Errorf("expected 5 decoders, got %d", len(m.decoders))
		}
	})

	t.Run("add custom decoder", func(t *testing.T) {
		m := NewDecoderManager()
		m.AddDecoder(&Base64Decoder{})
		if len(m.decoders) != 6 {
			t.Errorf("expected 6 decoders after add, got %d", len(m.decoders))
		}
	})

	t.Run("DetectAndDecode base64", func(t *testing.T) {
		m := NewDecoderManager()
		decoded, name, err := m.DetectAndDecode("SGVsbG8=")
		if err != nil {
			t.Fatalf("detect and decode failed: %v", err)
		}
		if name != "base64" {
			t.Errorf("expected 'base64', got %q", name)
		}
		if decoded != "Hello" {
			t.Errorf("expected 'Hello', got %q", decoded)
		}
	})

	t.Run("DetectAndDecode hex", func(t *testing.T) {
		m := NewDecoderManager()
		decoded, name, err := m.DetectAndDecode("48656c6c6f")
		if err != nil {
			t.Fatalf("detect and decode failed: %v", err)
		}
		if name != "hex" {
			t.Errorf("expected 'hex', got %q", name)
		}
		if decoded != "Hello" {
			t.Errorf("expected 'Hello', got %q", decoded)
		}
	})

	t.Run("DetectAndDecode no suitable decoder", func(t *testing.T) {
		m := NewDecoderManager()
		_, _, err := m.DetectAndDecode("plain text with no encoding at all")
		if err == nil {
			t.Error("expected error for plain text")
		}
	})

	t.Run("TryDecode returns original on failure", func(t *testing.T) {
		m := NewDecoderManager()
		result := m.TryDecode("plain text")
		if result != "plain text" {
			t.Errorf("expected 'plain text', got %q", result)
		}
	})

	t.Run("TryDecode decodes when possible", func(t *testing.T) {
		m := NewDecoderManager()
		result := m.TryDecode("SGVsbG8=")
		if result != "Hello" {
			t.Errorf("expected 'Hello', got %q", result)
		}
	})
}

// --- MultiDecode Tests ---

func TestMultiDecode(t *testing.T) {
	t.Run("default max attempts", func(t *testing.T) {
		m := NewDecoderManager()
		// Double base64 encoded: SGVsbG8= -> Hello -> (base64 of "Hello")
		doubleEncoded := "SGVsbG8="
		result, steps, err := m.MultiDecode(doubleEncoded, 0)
		// Should decode at least once
		if err != nil {
			t.Fatalf("multi decode failed: %v", err)
		}
		if len(steps) < 1 {
			t.Error("expected at least 1 decoding step")
		}
		if result != "Hello" {
			t.Errorf("expected 'Hello', got %q", result)
		}
	})

	t.Run("no decoding performed", func(t *testing.T) {
		m := NewDecoderManager()
		_, _, err := m.MultiDecode("plain text", 5)
		if err == nil {
			t.Error("expected error for plain text")
		}
	})

	t.Run("multiple attempts stops when no more decoding", func(t *testing.T) {
		m := NewDecoderManager()
		result, steps, err := m.MultiDecode("SGVsbG8=", 10)
		if err != nil {
			t.Fatalf("multi decode failed: %v", err)
		}
		// "SGVsbG8=" decodes to "Hello" which is plain text, so only 1 step
		if len(steps) != 1 {
			t.Errorf("expected 1 step, got %d: %v", len(steps), steps)
		}
		if result != "Hello" {
			t.Errorf("expected 'Hello', got %q", result)
		}
	})
}

// --- Standalone Functions Tests ---

func TestDetectEncoding(t *testing.T) {
	t.Run("detects base64", func(t *testing.T) {
		enc := DetectEncoding("SGVsbG8=")
		if enc != "base64" {
			t.Errorf("expected 'base64', got %q", enc)
		}
	})

	t.Run("detects hex", func(t *testing.T) {
		enc := DetectEncoding("48656c6c6f")
		if enc != "hex" {
			t.Errorf("expected 'hex', got %q", enc)
		}
	})

	t.Run("returns unknown for plain text", func(t *testing.T) {
		enc := DetectEncoding("plain text")
		if enc != "unknown" {
			t.Errorf("expected 'unknown', got %q", enc)
		}
	})
}

func TestAutoDecode(t *testing.T) {
	t.Run("decodes single encoding", func(t *testing.T) {
		result, steps, err := AutoDecode("SGVsbG8=")
		if err != nil {
			t.Fatalf("auto decode failed: %v", err)
		}
		if len(steps) != 1 {
			t.Errorf("expected 1 step, got %d", len(steps))
		}
		if result != "Hello" {
			t.Errorf("expected 'Hello', got %q", result)
		}
	})

	t.Run("returns error for plain text", func(t *testing.T) {
		_, _, err := AutoDecode("plain text")
		if err == nil {
			t.Error("expected error for plain text")
		}
	})
}

// --- Concurrent Access Tests ---

func TestDecoderConcurrency(t *testing.T) {
	m := NewDecoderManager()
	done := make(chan bool)

	for i := 0; i < 20; i++ {
		go func(id int) {
			data := "SGVsbG8="
			m.DetectAndDecode(data)
			m.TryDecode(data)
			m.MultiDecode(data, 5)
			DetectEncoding(data)
			AutoDecode(data)
			done <- true
		}(i)
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}
