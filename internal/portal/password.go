package portal

import (
	"fmt"
	"math/big"
	"strings"
	"unicode/utf8"
)

const publicModulus = "94dd2a8675fb779e6b9f7103698634cd400f27a154afa67af6166a43fc26417222a79506d34cacc7641946abda1785b7acf9910ad6a0978c91ec84d40b71d2891379af19ffb333e7517e390bd26ac312fe940c340466b4a5d4af1d65c3b5944078f96a1a51a5a53e4bc302818b7c9f63c4a1b07bd7d874cef1c3d4b2f5eb7871"

// EncryptPassword reproduces shu-net-keeper/src/rsa.rs, including its chunk size
// and zero padding. This portal protocol does not use PKCS#1 or OAEP padding.
func EncryptPassword(passwordWithMAC string) (string, error) {
	if !utf8.ValidString(passwordWithMAC) {
		return "", fmt.Errorf("密码必须是有效的 UTF-8 字符串")
	}
	n, ok := new(big.Int).SetString(publicModulus, 16)
	if !ok {
		return "", fmt.Errorf("无效的校园网 RSA 公钥")
	}
	e := big.NewInt(65537)
	chunkSize := 2 * ((n.BitLen()+7)/8 - 1)

	// Rust reverses Unicode scalar values before converting to UTF-8 bytes.
	runes := []rune(passwordWithMAC)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	data := []byte(string(runes))
	if remainder := len(data) % chunkSize; remainder != 0 {
		data = append(data, make([]byte, chunkSize-remainder)...)
	}

	parts := make([]string, 0, len(data)/chunkSize)
	for start := 0; start < len(data); start += chunkSize {
		// The Rust implementation builds little-endian integers; SetBytes uses
		// big-endian, so reverse each padded block before importing it.
		block := data[start : start+chunkSize]
		for i, j := 0, len(block)-1; i < j; i, j = i+1, j-1 {
			block[i], block[j] = block[j], block[i]
		}
		value := new(big.Int).SetBytes(block)
		parts = append(parts, new(big.Int).Exp(value, e, n).Text(16))
	}
	return strings.Join(parts, " "), nil
}
