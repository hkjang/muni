package pdfx

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
)

var passwordPad = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41, 0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80, 0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

type decryptor struct {
	key             []byte
	useAES          bool
	revision        int
	identityStreams bool
	identityStrings bool
}

// setupDecryption unlocks documents that carry only an owner password, which
// is by far the most common form of "protected" PDF users try to import.
func (d *Document) setupDecryption() error {
	encryptRef := d.trailer.get("Encrypt")
	if encryptRef == nil {
		return nil
	}
	encrypt := d.dict(encryptRef)
	if encrypt == nil {
		return nil
	}
	if filter := d.name(encrypt.get("Filter")); filter != "" && filter != "Standard" {
		return errors.New("지원하지 않는 PDF 암호화 방식입니다")
	}
	version, _ := toInt(d.resolve(encrypt.get("V")))
	revision, _ := toInt(d.resolve(encrypt.get("R")))
	lengthBits, ok := toInt(d.resolve(encrypt.get("Length")))
	if !ok || lengthBits == 0 {
		lengthBits = 40
	}
	ownerBytes := bytesOf(d.resolve(encrypt.get("O")))
	userBytes := bytesOf(d.resolve(encrypt.get("U")))
	permissions, _ := toInt(d.resolve(encrypt.get("P")))
	metadataEncrypted := true
	if value, ok := d.resolve(encrypt.get("EncryptMetadata")).(bool); ok {
		metadataEncrypted = value
	}

	useAES := false
	identityStreams := false
	identityStrings := false
	if version >= 4 {
		filters := d.dict(encrypt.get("CF"))
		streamFilter := d.name(encrypt.get("StmF"))
		stringFilter := d.name(encrypt.get("StrF"))
		if streamFilter == "" {
			streamFilter = "Identity"
		}
		if stringFilter == "" {
			stringFilter = "Identity"
		}
		identityStreams = streamFilter == "Identity"
		identityStrings = stringFilter == "Identity"
		resolveMethod := func(name string) string {
			if name == "Identity" || filters == nil {
				return "Identity"
			}
			entry := d.dict(filters.get(Name(name)))
			if entry == nil {
				return "Identity"
			}
			if bits, ok := toInt(d.resolve(entry.get("Length"))); ok && bits > 0 {
				if bits <= 40 {
					lengthBits = bits * 8
				} else {
					lengthBits = bits
				}
			}
			return d.name(entry.get("CFM"))
		}
		method := resolveMethod(streamFilter)
		if method == "Identity" {
			method = resolveMethod(stringFilter)
		}
		switch method {
		case "AESV2":
			useAES = true
			lengthBits = 128
		case "AESV3":
			useAES = true
			lengthBits = 256
		}
	}
	if version == 5 || revision >= 5 {
		useAES = true
		lengthBits = 256
	}

	var key []byte
	if revision >= 5 {
		derived, err := unlockV5(userBytes, bytesOf(d.resolve(encrypt.get("UE"))))
		if err != nil {
			return err
		}
		key = derived
	} else {
		identifier := firstID(d, d.trailer.get("ID"))
		key = deriveKeyLegacy(ownerBytes, permissions, identifier, revision, lengthBits/8, metadataEncrypted)
	}
	if len(key) == 0 {
		return errors.New("암호로 보호된 PDF는 가져올 수 없습니다")
	}
	d.decryptor = &decryptor{key: key, useAES: useAES, revision: revision, identityStreams: identityStreams, identityStrings: identityStrings}
	// The encrypt dictionary itself is never encrypted; drop cached copies so
	// objects parsed before the key existed get decrypted on the next read.
	d.cache = map[int]Object{}
	return nil
}

func firstID(d *Document, value Object) []byte {
	array := d.array(value)
	if len(array) == 0 {
		return nil
	}
	return bytesOf(d.resolve(array[0]))
}

func bytesOf(value Object) []byte {
	switch typed := value.(type) {
	case String:
		return []byte(typed)
	case Name:
		return []byte(typed)
	}
	return nil
}

func deriveKeyLegacy(owner []byte, permissions int, identifier []byte, revision, keyLength int, metadataEncrypted bool) []byte {
	if keyLength <= 0 || keyLength > 16 {
		keyLength = 5
	}
	if revision == 2 {
		keyLength = 5
	}
	hash := md5.New()
	hash.Write(passwordPad)
	padded := make([]byte, 32)
	copy(padded, owner)
	hash.Write(padded)
	hash.Write([]byte{byte(permissions), byte(permissions >> 8), byte(permissions >> 16), byte(permissions >> 24)})
	hash.Write(identifier)
	if revision >= 4 && !metadataEncrypted {
		hash.Write([]byte{0xff, 0xff, 0xff, 0xff})
	}
	sum := hash.Sum(nil)
	if revision >= 3 {
		for round := 0; round < 50; round++ {
			next := md5.Sum(sum[:keyLength])
			sum = next[:]
		}
	}
	return sum[:keyLength]
}

// unlockV5 validates the empty user password for AES-256 documents and
// recovers the file key from /UE.
func unlockV5(user, userEncrypted []byte) ([]byte, error) {
	if len(user) < 48 || len(userEncrypted) < 32 {
		return nil, errors.New("암호로 보호된 PDF는 가져올 수 없습니다")
	}
	validationSalt := user[32:40]
	keySalt := user[40:48]
	hash := hash2B(nil, validationSalt, nil)
	if !bytes.Equal(hash, user[:32]) {
		return nil, errors.New("사용자 암호가 필요한 PDF는 가져올 수 없습니다")
	}
	intermediate := hash2B(nil, keySalt, nil)
	block, err := aes.NewCipher(intermediate)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 32)
	cipher.NewCBCDecrypter(block, make([]byte, 16)).CryptBlocks(out, userEncrypted[:32])
	return out, nil
}

// hash2B is the SHA-2 based password hash from ISO 32000-2 (PDF 2.0), which
// revision 6 documents use.
func hash2B(password, salt, extra []byte) []byte {
	input := append(append(append([]byte{}, password...), salt...), extra...)
	sum := sha256.Sum256(input)
	digest := sum[:]
	for round := 0; ; round++ {
		block := append(append([]byte{}, password...), digest...)
		block = append(block, extra...)
		repeated := bytes.Repeat(block, 64)
		aesKey, err := aes.NewCipher(digest[:16])
		if err != nil {
			return digest
		}
		encrypted := make([]byte, len(repeated))
		cipher.NewCBCEncrypter(aesKey, digest[16:32]).CryptBlocks(encrypted, repeated)
		total := 0
		for _, value := range encrypted[:16] {
			total += int(value)
		}
		switch total % 3 {
		case 0:
			next := sha256.Sum256(encrypted)
			digest = next[:]
		case 1:
			next := sha512.Sum384(encrypted)
			digest = next[:]
		case 2:
			next := sha512.Sum512(encrypted)
			digest = next[:]
		}
		if round >= 63 && int(encrypted[len(encrypted)-1]) <= round-32 {
			break
		}
	}
	return digest[:32]
}

func (c *decryptor) objectKey(reference Ref) []byte {
	if c.revision >= 5 {
		return c.key
	}
	input := append([]byte{}, c.key...)
	input = append(input,
		byte(reference.Number), byte(reference.Number>>8), byte(reference.Number>>16),
		byte(reference.Generation), byte(reference.Generation>>8))
	if c.useAES {
		input = append(input, 0x73, 0x41, 0x6c, 0x54)
	}
	sum := md5.Sum(input)
	length := len(c.key) + 5
	if length > 16 {
		length = 16
	}
	return sum[:length]
}

func (c *decryptor) decrypt(data []byte, reference Ref, isString bool) []byte {
	if len(data) == 0 {
		return data
	}
	if isString && c.identityStrings {
		return data
	}
	key := c.objectKey(reference)
	if c.useAES {
		if len(data) <= 16 {
			return data
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return data
		}
		body := data[16:]
		body = body[:len(body)-len(body)%16]
		if len(body) == 0 {
			return nil
		}
		out := make([]byte, len(body))
		cipher.NewCBCDecrypter(block, data[:16]).CryptBlocks(out, body)
		if padding := int(out[len(out)-1]); padding > 0 && padding <= 16 && padding <= len(out) {
			out = out[:len(out)-padding]
		}
		return out
	}
	stream, err := rc4.NewCipher(key)
	if err != nil {
		return data
	}
	out := make([]byte, len(data))
	stream.XORKeyStream(out, data)
	return out
}

// decryptObject walks an object graph decrypting every string it contains.
func (d *Document) decryptObject(value Object, reference Ref) Object {
	if d.decryptor == nil {
		return value
	}
	switch typed := value.(type) {
	case String:
		return String(d.decryptor.decrypt([]byte(typed), reference, true))
	case Array:
		for index := range typed {
			typed[index] = d.decryptObject(typed[index], reference)
		}
		return typed
	case Dict:
		for key := range typed {
			typed[key] = d.decryptObject(typed[key], reference)
		}
		return typed
	}
	return value
}
