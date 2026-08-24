package pdfx

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

const maxExtractedImagePixels = 40 << 20

// imageBytes turns an image XObject into a file a browser can display: JPEG
// payloads pass through untouched, everything else is re-encoded as PNG.
func (d *Document) imageBytes(stream *Stream) ([]byte, string, bool) {
	filters := namesOf(d.resolve(stream.Dict.get("Filter", "F")))
	last := ""
	if len(filters) > 0 {
		last = filters[len(filters)-1]
	}
	switch last {
	case "DCTDecode", "DCT":
		data, err := d.StreamData(stream)
		if err != nil || len(data) < 4 {
			return nil, "", false
		}
		return data, "image/jpeg", true
	case "JPXDecode", "JBIG2Decode", "CCITTFaxDecode", "CCF":
		// No decoder available; skipping beats embedding an unreadable blob.
		return nil, "", false
	}

	width, _ := toInt(d.resolve(stream.Dict.get("Width", "W")))
	height, _ := toInt(d.resolve(stream.Dict.get("Height", "H")))
	if width <= 0 || height <= 0 || width*height > maxExtractedImagePixels {
		return nil, "", false
	}
	bits, ok := toInt(d.resolve(stream.Dict.get("BitsPerComponent", "BPC")))
	if !ok || bits == 0 {
		bits = 8
	}
	data, err := d.StreamData(stream)
	if err != nil || len(data) == 0 {
		return nil, "", false
	}
	components, space := d.colorComponents(stream)
	if components == 0 {
		return nil, "", false
	}
	if mask, _ := d.resolve(stream.Dict.get("ImageMask", "IM")).(bool); mask {
		components, bits, space = 1, 1, "DeviceGray"
	}

	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	rowBits := width * components * bits
	rowBytes := (rowBits + 7) / 8
	if len(data) < rowBytes*height {
		return nil, "", false
	}
	maxValue := float64(int(1)<<uint(bits)) - 1
	sample := func(row, index int) float64 {
		bitOffset := index * bits
		switch bits {
		case 8:
			return float64(data[row*rowBytes+bitOffset/8])
		case 16:
			return float64(data[row*rowBytes+bitOffset/8])*256 + float64(data[row*rowBytes+bitOffset/8+1])
		default:
			position := row*rowBytes*8 + bitOffset
			value := 0
			for count := 0; count < bits; count++ {
				byteIndex := (position + count) / 8
				if byteIndex >= len(data) {
					return 0
				}
				bitIndex := 7 - uint((position+count)%8)
				value = value<<1 | int((data[byteIndex]>>bitIndex)&1)
			}
			return float64(value)
		}
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			base := x * components
			var rgba color.NRGBA
			switch {
			case components >= 3:
				rgba = color.NRGBA{
					R: uint8(sample(y, base) / maxValue * 255),
					G: uint8(sample(y, base+1) / maxValue * 255),
					B: uint8(sample(y, base+2) / maxValue * 255),
					A: 255,
				}
				if space == "DeviceCMYK" && components >= 4 {
					c := sample(y, base) / maxValue
					mVal := sample(y, base+1) / maxValue
					yVal := sample(y, base+2) / maxValue
					k := sample(y, base+3) / maxValue
					rgba = color.NRGBA{
						R: uint8((1 - min(1, c+k)) * 255),
						G: uint8((1 - min(1, mVal+k)) * 255),
						B: uint8((1 - min(1, yVal+k)) * 255),
						A: 255,
					}
				}
			default:
				level := uint8(sample(y, base) / maxValue * 255)
				rgba = color.NRGBA{R: level, G: level, B: level, A: 255}
			}
			picture.SetNRGBA(x, y, rgba)
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		return nil, "", false
	}
	return buffer.Bytes(), "image/png", true
}

func (d *Document) colorComponents(stream *Stream) (int, string) {
	space := d.resolve(stream.Dict.get("ColorSpace", "CS"))
	switch typed := space.(type) {
	case Name:
		switch string(typed) {
		case "DeviceGray", "CalGray", "G":
			return 1, "DeviceGray"
		case "DeviceRGB", "CalRGB", "RGB":
			return 3, "DeviceRGB"
		case "DeviceCMYK", "CMYK":
			return 4, "DeviceCMYK"
		}
		return 0, ""
	case Array:
		if len(typed) == 0 {
			return 0, ""
		}
		switch d.name(typed[0]) {
		case "ICCBased":
			if len(typed) > 1 {
				if iccStream, ok := d.resolve(typed[1]).(*Stream); ok {
					if count, ok := toInt(d.resolve(iccStream.Dict.get("N"))); ok {
						switch count {
						case 1:
							return 1, "DeviceGray"
						case 3:
							return 3, "DeviceRGB"
						case 4:
							return 4, "DeviceCMYK"
						}
					}
				}
			}
		case "Indexed", "I", "Separation", "DeviceN":
			return 0, ""
		case "CalGray":
			return 1, "DeviceGray"
		case "CalRGB", "Lab":
			return 3, "DeviceRGB"
		}
	}
	return 0, ""
}
