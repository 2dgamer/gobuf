package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"unicode"

	"github.com/funny/gobuf/parser"

	"strings"
)

func LcFirst(str string) string {
	for i, v := range str {
		return string(unicode.ToLower(v)) + str[i+1:]
	}
	return ""
}

func main() {
	var doc parser.Doc

	decoder := json.NewDecoder(os.Stdin)

	if err := decoder.Decode(&doc); err != nil {
		log.Fatal(err)
	}

	var o writer

	o.Writef("using System;")
	o.Writef("using System.Collections.Generic;")

	o.Writef("namespace FastNet {")
	o.Writef("class %s {", strings.Title(doc.Package))
	o.Writef("private FastClient client;")

	o.Writef("enum MessageID : byte {")
	autoMsgID := 0
	var lastMsgName string
	var msgNames []string
	for _, s := range doc.Structs {
		if strings.ToLower(s.Name) == doc.Package {
			continue
		}

		curMsgName := s.Name[0 : len(s.Name)-3]
		if curMsgName != lastMsgName {
			o.Writef("MsgID_%s = %d", curMsgName, autoMsgID)
			autoMsgID++
			msgNames = append(msgNames, curMsgName)
		}
		lastMsgName = curMsgName
	}
	o.Writef("};")

	for _, s := range doc.Structs {
		if s.Name[len(s.Name)-3:] == "Rsp" {
			o.Writef("public delegate void %sHandler(%s %s);", s.Name, s.Name, LcFirst(s.Name))
			o.Writef("public %sHandler %sHandle;", s.Name, LcFirst(s.Name))
		}
	}

	o.Writef("public %s(FastClient c) {", strings.Title(doc.Package))
	o.Writef("this.client = c;")
	for _, s := range doc.Structs {
		if s.Name[len(s.Name)-3:] == "Rsp" {
			o.Writef("this.client.registHandle ((byte)ServiceID.ServiceID_%s, (byte)Module1.MessageID.MsgID_%s, this.handle%s);", strings.Title(doc.Package), s.Name[0:len(s.Name)-3], s.Name)
		}
	}
	o.Writef("}")

	for _, s := range doc.Structs {
		if s.Name[len(s.Name)-3:] == "Rsp" {
			o.Writef("public void handle%s(byte[] content) {", s.Name)
			o.Writef("%s %s = new %s ();", s.Name, LcFirst(s.Name), s.Name)
			o.Writef("%s.Unmarshal (content, 0);", LcFirst(s.Name))
			o.Writef("if (%sHandle != null) {", LcFirst(s.Name))
			o.Writef("%sHandle (%s);", LcFirst(s.Name), LcFirst(s.Name))
			o.Writef("}")
			o.Writef("}")

		} else if s.Name[len(s.Name)-3:] == "Req" {
			o.Writef("public void Send%s(%s %s) {", s.Name, s.Name, LcFirst(s.Name))
			o.Writef("byte[] b = new byte[%s.Size()];", LcFirst(s.Name))
			o.Writef("%s.Marshal (b, 0);", LcFirst(s.Name))
			o.Writef("client.Send((byte)ServiceID.ServiceID_%s, (byte)MessageID.MsgID_%s, b);", strings.Title(doc.Package), s.Name[0:len(s.Name)-3])
			o.Writef("}")
		}
	}

	for _, s := range doc.Structs {
		if strings.ToLower(s.Name) == doc.Package {
			continue
		}

		o.Writef("public class %s {", s.Name)

		for _, field := range s.Fields {
			if field.Type.Kind == parser.ARRAY {
				if field.Type.Len != 0 {
					o.Writef("public %s %s = new %s[%d];",
						typeName(field.Type), field.Name, typeName(field.Type.Elem), field.Type.Len)
				} else {
					o.Writef("public %s %s = new %s();",
						typeName(field.Type), field.Name, typeName(field.Type))
				}
			} else if field.Type.Kind == parser.MAP {
				o.Writef("public %s %s = new %s();",
					typeName(field.Type), field.Name, typeName(field.Type))
			} else if field.Type.Kind == parser.BYTES && field.Type.Len != 0 {
				o.Writef("public %s %s = new byte[%d];",
					typeName(field.Type), field.Name, field.Type.Len)
			} else {
				o.Writef("public %s %s;", typeName(field.Type), field.Name)
			}
		}

		o.Writef("public int Size() {")
		o.Writef("int size = 0;")
		for _, field := range s.Fields {
			genSizer(&o, "this."+field.Name, field.Type, 1)
		}
		o.Writef("return size;")
		o.Writef("}")

		o.Writef("public int Marshal(byte[] b, int n) {")
		for _, field := range s.Fields {
			genMarshaler(&o, "this."+field.Name, field.Type, 1)
		}
		o.Writef("return n;")
		o.Writef("}")

		o.Writef("public int Unmarshal(byte[] b, int n) {")
		for _, field := range s.Fields {
			genUnmarshaler(&o, "this."+field.Name, field.Type, 1)
		}
		o.Writef("return n;")
		o.Writef("}")

		o.Writef("}")
	}
	o.Writef("}")
	o.Writef("}")

	if _, err := o.WriteTo(os.Stdout); err != nil {
		log.Fatal(err)
	}
}

type writer struct {
	deepth int
	bytes.Buffer
}

func (w *writer) Writef(format string, args ...interface{}) {
	format = strings.TrimLeft(format, "\t ")

	if format[0] == '}' {
		w.deepth--
	}

	for i := 0; i < w.deepth; i++ {
		w.WriteByte('\t')
	}

	if format[len(format)-1] == '{' {
		w.deepth++
	}

	w.WriteString(fmt.Sprintf(format, args...))
	w.WriteByte('\n')
}

func isNullable(t *parser.Type) bool {
	return t.Kind == parser.POINTER && t.Elem.Kind != parser.STRUCT && t.Elem.Kind != parser.STRING
}

func typeName(t *parser.Type) string {
	if t.Name != "" {
		return t.Name
	}
	switch t.Kind {
	case parser.INT:
		return "long"
	case parser.UINT:
		return "ulong"
	case parser.INT8:
		return "sbyte"
	case parser.UINT8:
		return "byte"
	case parser.INT16:
		return "short"
	case parser.UINT16:
		return "ushort"
	case parser.INT32:
		return "int"
	case parser.UINT32:
		return "uint"
	case parser.INT64:
		return "long"
	case parser.UINT64:
		return "ulong"
	case parser.FLOAT32:
		return "float"
	case parser.FLOAT64:
		return "double"
	case parser.STRING:
		return "string"
	case parser.BYTES:
		return "byte[]"
	case parser.BOOL:
		return "bool"
	case parser.MAP:
		return fmt.Sprintf("Dictionary<%s, %s>", typeName(t.Key), typeName(t.Elem))
	case parser.POINTER:
		if t.Elem.Kind == parser.STRUCT {
			return typeName(t.Elem)
		}
		if t.Elem.Kind == parser.STRING {
			return "string"
		}
		return fmt.Sprintf("Nullable<%s>", typeName(t.Elem))
	case parser.ARRAY:
		if t.Len != 0 {
			return fmt.Sprintf("%s[]", typeName(t.Elem))
		}
		return fmt.Sprintf("List<%s>", typeName(t.Elem))
	}
	return ""
}
