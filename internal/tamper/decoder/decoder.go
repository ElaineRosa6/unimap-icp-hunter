package decoder

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Decoder 解码接口
type Decoder interface {
	Decode(data string) (string, error)
	CanDecode(data string) bool
	Name() string
}

// DecoderManager 解码器管理器
type DecoderManager struct {
	decoders []Decoder
}

// NewDecoderManager 创建解码器管理器
func NewDecoderManager() *DecoderManager {
	return &DecoderManager{
		decoders: []Decoder{
			&Base64Decoder{},
			&HexDecoder{},
			&UnicodeDecoder{},
			&URLDecoder{},
			&HTMLDecoder{},
		},
	}
}

// AddDecoder 添加解码器
func (m *DecoderManager) AddDecoder(decoder Decoder) {
	m.decoders = append(m.decoders, decoder)
}

// DetectAndDecode 检测并解码数据
func (m *DecoderManager) DetectAndDecode(data string) (string, string, error) {
	for _, decoder := range m.decoders {
		if decoder.CanDecode(data) {
			decoded, err := decoder.Decode(data)
			if err == nil {
				return decoded, decoder.Name(), nil
			}
		}
	}
	return "", "", fmt.Errorf("no suitable decoder found")
}

// TryDecode 尝试解码数据（忽略错误）
func (m *DecoderManager) TryDecode(data string) string {
	for _, decoder := range m.decoders {
		if decoder.CanDecode(data) {
			if decoded, err := decoder.Decode(data); err == nil {
				return decoded
			}
		}
	}
	return data
}

// MultiDecode 尝试多次解码（处理多重编码）
func (m *DecoderManager) MultiDecode(data string, maxAttempts int) (string, []string, error) {
	if maxAttempts<= 0 {
		maxAttempts = 5
	}
	
	var decodingSteps []string
	currentData := data
	
	for i := 0; i < maxAttempts; i++ {
		decoded, decoderName, err := m.DetectAndDecode(currentData)
		if err != nil {
			break
		}
		
		if decoded == currentData {
			break
		}
		
		decodingSteps = append(decodingSteps, decoderName)
		currentData = decoded
	}
	
	if len(decodingSteps) == 0 {
		return data, nil, fmt.Errorf("no decoding performed")
	}
	
	return currentData, decodingSteps, nil
}

// Base64Decoder Base64解码器
type Base64Decoder struct{}

func (d *Base64Decoder) Name() string {
	return "base64"
}

func (d *Base64Decoder) CanDecode(data string) bool {
	data = strings.TrimSpace(data)
	if len(data) == 0 {
		return false
	}
	
	// 检查是否符合Base64格式
	if len(data)%4 != 0 {
		return false
	}
	
	// 检查是否只包含Base64字符
	matched, _ := regexp.MatchString(`^[A-Za-z0-9+/]*={0,2}$`, data)
	return matched
}

func (d *Base64Decoder) Decode(data string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}
	return string(decoded), nil
}

// HexDecoder Hex解码器
type HexDecoder struct{}

func (d *HexDecoder) Name() string {
	return "hex"
}

func (d *HexDecoder) CanDecode(data string) bool {
	data = strings.TrimSpace(data)
	if len(data) == 0 || len(data)%2 != 0 {
		return false
	}
	
	matched, _ := regexp.MatchString(`^[0-9a-fA-F]+$`, data)
	return matched
}

func (d *HexDecoder) Decode(data string) (string, error) {
	decoded, err := hex.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("hex decode failed: %w", err)
	}
	return string(decoded), nil
}

// UnicodeDecoder Unicode解码器
type UnicodeDecoder struct{}

func (d *UnicodeDecoder) Name() string {
	return "unicode"
}

func (d *UnicodeDecoder) CanDecode(data string) bool {
	// 检测Unicode转义序列 (\uXXXX 或 \UXXXXXXXX)
	matched, _ := regexp.MatchString(`\\u[0-9a-fA-F]{4}`, data)
	return matched
}

func (d *UnicodeDecoder) Decode(data string) (string, error) {
	re := regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)
	return re.ReplaceAllStringFunc(data, func(match string) string {
		if len(match) != 6 {
			return match
		}
		
		codePoint, err := strconv.ParseUint(match[2:], 16, 16)
		if err != nil {
			return match
		}
		
		r := rune(codePoint)
		if r == 0 {
			return match
		}
		
		return string(r)
	}), nil
}

// URLDecoder URL解码器
type URLDecoder struct{}

func (d *URLDecoder) Name() string {
	return "url"
}

func (d *URLDecoder) CanDecode(data string) bool {
	// 检测URL编码字符
	matched, _ := regexp.MatchString(`%[0-9a-fA-F]{2}`, data)
	return matched
}

func (d *URLDecoder) Decode(data string) (string, error) {
	re := regexp.MustCompile(`%([0-9a-fA-F]{2})`)
	return re.ReplaceAllStringFunc(data, func(match string) string {
		if len(match) != 3 {
			return match
		}
		
		byteValue, err := strconv.ParseUint(match[1:], 16, 8)
		if err != nil {
			return match
		}
		
		return string(byte(byteValue))
	}), nil
}

// HTMLDecoder HTML实体解码器
type HTMLDecoder struct{}

func (d *HTMLDecoder) Name() string {
	return "html"
}

func (d *HTMLDecoder) CanDecode(data string) bool {
	// 检测HTML实体
	matched, _ := regexp.MatchString(`&[a-zA-Z]+;`, data)
	return matched
}

func (d *HTMLDecoder) Decode(data string) (string, error) {
	// 简单的HTML实体解码
	data = strings.ReplaceAll(data, "&amp;", "&")
	data = strings.ReplaceAll(data, "&lt;", "<")
	data = strings.ReplaceAll(data, "&gt;", ">")
	data = strings.ReplaceAll(data, "&quot;", "\"")
	data = strings.ReplaceAll(data, "&#39;", "'")
	
	// 处理数字实体
	re := regexp.MustCompile(`&#(\d+);`)
	data = re.ReplaceAllStringFunc(data, func(match string) string {
		if len(match) < 3 {
			return match
		}
		
		codePoint, err := strconv.ParseUint(match[2:len(match)-1], 10, 32)
		if err != nil {
			return match
		}
		
		r := rune(codePoint)
		if !utf8.ValidRune(r) {
			return match
		}
		
		return string(r)
	})
	
	return data, nil
}

// DetectEncoding 检测编码类型
func DetectEncoding(data string) string {
	manager := NewDecoderManager()
	_, decoderName, err := manager.DetectAndDecode(data)
	if err == nil {
		return decoderName
	}
	return "unknown"
}

// AutoDecode 自动解码（支持多重编码）
func AutoDecode(data string) (string, []string, error) {
	manager := NewDecoderManager()
	return manager.MultiDecode(data, 10)
}