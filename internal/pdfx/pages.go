package pdfx

type pageRef struct {
	dict      Dict
	rotate    int
	mediaBox  [4]float64
	resources Dict
}

const maxPages = 2000

// pages walks the page tree, carrying the attributes that pages inherit from
// their parent nodes.
func (d *Document) pages() []pageRef {
	catalog := d.catalog()
	out := make([]pageRef, 0, 16)
	seen := map[int]bool{}

	var walk func(node Object, inherited pageRef, depth int)
	walk = func(node Object, inherited pageRef, depth int) {
		if depth > 64 || len(out) >= maxPages {
			return
		}
		if reference, ok := node.(Ref); ok {
			if seen[reference.Number] {
				return
			}
			seen[reference.Number] = true
		}
		dict := d.dict(node)
		if dict == nil {
			return
		}
		current := inherited
		if box := d.rectangle(dict.get("MediaBox")); box != nil {
			current.mediaBox = *box
		}
		if value, ok := toInt(d.resolve(dict.get("Rotate"))); ok {
			current.rotate = value
		}
		if resources := d.dict(dict.get("Resources")); resources != nil {
			current.resources = resources
		}
		kids := d.array(dict.get("Kids"))
		if d.name(dict.get("Type")) == "Page" || (len(kids) == 0 && dict.get("Contents") != nil) {
			current.dict = dict
			if current.resources != nil && dict.get("Resources") == nil {
				dict["Resources"] = current.resources
			}
			if current.mediaBox[2] <= current.mediaBox[0] || current.mediaBox[3] <= current.mediaBox[1] {
				current.mediaBox = [4]float64{0, 0, 595, 842}
			}
			out = append(out, current)
			return
		}
		for _, kid := range kids {
			walk(kid, current, depth+1)
		}
	}

	root := Object(nil)
	if catalog != nil {
		root = catalog.get("Pages")
	}
	if root == nil {
		// Damaged catalog: fall back to every object that looks like a page.
		for number := range d.offsets {
			if dict, ok := d.loadAt(number).(Dict); ok && d.name(dict.get("Type")) == "Page" {
				walk(dict, pageRef{mediaBox: [4]float64{0, 0, 595, 842}}, 0)
			}
		}
		return out
	}
	walk(root, pageRef{mediaBox: [4]float64{0, 0, 595, 842}}, 0)
	if len(out) == 0 {
		for number := range d.offsets {
			if dict, ok := d.loadAt(number).(Dict); ok && d.name(dict.get("Type")) == "Page" {
				walk(dict, pageRef{mediaBox: [4]float64{0, 0, 595, 842}}, 0)
			}
		}
	}
	return out
}

func (d *Document) rectangle(value Object) *[4]float64 {
	array := d.array(value)
	if len(array) != 4 {
		return nil
	}
	var box [4]float64
	for index := 0; index < 4; index++ {
		number, ok := toFloat(d.resolve(array[index]))
		if !ok {
			return nil
		}
		box[index] = number
	}
	if box[0] > box[2] {
		box[0], box[2] = box[2], box[0]
	}
	if box[1] > box[3] {
		box[1], box[3] = box[3], box[1]
	}
	return &box
}

// Title reads the document title from the info dictionary or XMP catalog.
func (d *Document) Title() string {
	info := d.dict(d.trailer.get("Info"))
	if info == nil {
		return ""
	}
	switch value := d.resolve(info.get("Title")).(type) {
	case String:
		return decodeTextString(value)
	}
	return ""
}

func decodeTextString(value String) string {
	if len(value) >= 2 && value[0] == 0xfe && value[1] == 0xff {
		return decodeUTF16BE(value[2:])
	}
	out := make([]rune, 0, len(value))
	for _, item := range value {
		if item == 0 {
			continue
		}
		if text := pdfDocEncoding(item); text != 0 {
			out = append(out, text)
		}
	}
	return string(out)
}

func pdfDocEncoding(code byte) rune {
	if code >= 32 && code < 127 {
		return rune(code)
	}
	if value := winAnsiEncoding[code]; value != "" {
		return []rune(value)[0]
	}
	return 0
}
