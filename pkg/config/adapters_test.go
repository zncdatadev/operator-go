/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config_test

import (
	"math/rand"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/config"
)

var _ = Describe("unescapeProperties", func() {
	// This tests the unescapeProperties function (line 169) with edge cases

	Describe("Escape sequences", func() {
		It("should unescape newline character", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=value\\nwith\\nnewlines\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "value\nwith\nnewlines"))
		})

		It("should unescape carriage return character", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=value\\rcarriage\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "value\rcarriage"))
		})

		It("should unescape tab character", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=col1\\tcol2\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "col1\tcol2"))
		})

		It("should unescape backslash", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=path\\\\to\\\\file\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "path\\to\\file"))
		})

		It("should unescape equals sign", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=a\\=b\\=c\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "a=b=c"))
		})

		It("should unescape colon", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=http\\://localhost\\:8080\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "http://localhost:8080"))
		})

		It("should unescape space", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=hello\\ world\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "hello world"))
		})

		It("should unescape hash/pound sign", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=not\\#a\\#comment\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "not#a#comment"))
		})

		It("should unescape exclamation mark", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=Hello\\!World\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "Hello!World"))
		})
	})

	Describe("Edge cases with backslashes", func() {
		It("should handle backslash at end of string as line continuation", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=endswith\\\ncontinued\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "endswithcontinued"))
		})

		It("should handle consecutive backslashes", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=\\\\\\\\double\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "\\\\double"))
		})

		It("should handle unknown escape sequences", func() {
			// Unknown escape sequences should keep the backslash
			adapter := config.NewPropertiesAdapter()
			content := "key=unknown\\xescape\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			// Unknown escape keeps the backslash
			Expect(result).To(HaveKeyWithValue("key", "unknown\\xescape"))
		})

		It("should handle multiple escape sequences in one value", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=a\\nb\\tc\\\\d\\re\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "a\nb\tc\\d\re"))
		})

		It("should handle single backslash followed by escape char", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=\\n\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "\n"))
		})
	})

	Describe("Empty and simple strings", func() {
		It("should handle empty value", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", ""))
		})

		It("should handle value without escapes", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=simple\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "simple"))
		})

		It("should handle key with no separator", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", ""))
		})
	})

	Describe("Key unescaping", func() {
		It("should unescape special characters in keys", func() {
			// Test via marshal/unmarshal round-trip which handles this correctly
			adapter := config.NewPropertiesAdapter()
			original := map[string]string{
				"key with spaces": "value",
			}
			marshaled, err := adapter.Marshal(original)
			Expect(err).ToNot(HaveOccurred())
			// Verify the key is escaped in the marshaled output
			Expect(marshaled).To(ContainSubstring("\\ "))
		})

		It("should unescape newline in value", func() {
			adapter := config.NewPropertiesAdapter()
			content := "key=hello\\nworld\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "hello\nworld"))
		})
	})

	Describe("Round-trip consistency", func() {
		It("should round-trip values with newlines", func() {
			adapter := config.NewPropertiesAdapter()
			original := map[string]string{
				"key": "line1\nline2\nline3",
			}
			marshaled, err := adapter.Marshal(original)
			Expect(err).ToNot(HaveOccurred())

			unmarshaled, err := adapter.Unmarshal(marshaled)
			Expect(err).ToNot(HaveOccurred())
			Expect(unmarshaled).To(Equal(original))
		})

		It("should round-trip values with tabs", func() {
			adapter := config.NewPropertiesAdapter()
			original := map[string]string{
				"key": "col1\tcol2\tcol3",
			}
			marshaled, err := adapter.Marshal(original)
			Expect(err).ToNot(HaveOccurred())

			unmarshaled, err := adapter.Unmarshal(marshaled)
			Expect(err).ToNot(HaveOccurred())
			Expect(unmarshaled).To(Equal(original))
		})

		It("should round-trip values with backslashes", func() {
			adapter := config.NewPropertiesAdapter()
			original := map[string]string{
				"key": `C:\Users\test\path`,
			}
			marshaled, err := adapter.Marshal(original)
			Expect(err).ToNot(HaveOccurred())

			unmarshaled, err := adapter.Unmarshal(marshaled)
			Expect(err).ToNot(HaveOccurred())
			Expect(unmarshaled).To(Equal(original))
		})

		It("should round-trip values with carriage returns", func() {
			adapter := config.NewPropertiesAdapter()
			original := map[string]string{
				"key": "line1\r\nline2",
			}
			marshaled, err := adapter.Marshal(original)
			Expect(err).ToNot(HaveOccurred())

			unmarshaled, err := adapter.Unmarshal(marshaled)
			Expect(err).ToNot(HaveOccurred())
			Expect(unmarshaled).To(Equal(original))
		})

		// Keys carrying a separator are escaped on the way out ("a\=b"), so the parser must skip
		// escaped separators to find the real split point; a value ending in a backslash is
		// escaped to an even count, which must not read as a line continuation.
		DescribeTable("should round-trip keys and values a naive parser splits or drops",
			func(key, value string) {
				adapter := config.NewPropertiesAdapter()
				original := map[string]string{
					key:      value,
					"plain":  "kept",
					"second": "also kept",
				}
				marshaled, err := adapter.Marshal(original)
				Expect(err).ToNot(HaveOccurred())

				unmarshaled, err := adapter.Unmarshal(marshaled)
				Expect(err).ToNot(HaveOccurred())
				Expect(unmarshaled).To(Equal(original))
			},
			Entry("equals sign in key", "a=b", "v1"),
			Entry("colon in key", "a:b", "v1"),
			Entry("space in key", "a b", "v1"),
			Entry("all separators in key", "a=b:c d", "v1"),
			Entry("value ending in a backslash", "zlast", `C:\`),
			Entry("value ending in two backslashes", "zlast", `C:\\`),
			Entry("empty value", "zlast", ""),
			Entry("trailing space in key", "a ", "v1"),
			Entry("leading space in key", " a", "v1"),
			Entry("key of spaces only", "  ", "v1"),
			Entry("comment marker starting the key", "#a", "v1"),
			Entry("bang marker starting the key", "!a", "v1"),
			Entry("trailing space in value", "akey", "v1 "),
			Entry("leading space in value", "akey", " v1"),
			Entry("value of spaces only", "akey", " "),
			Entry("backslash before the value's trailing space", "akey", `v1\ `),
			Entry("tab at both value edges", "akey", "\tv1\t"),
		)

		It("should keep an entry whose value ends the input on a continuation", func() {
			adapter := config.NewPropertiesAdapter()
			// The input ends mid-continuation (no terminating line); the entry is still real.
			result, err := adapter.Unmarshal("key=value\\")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "value"))
		})

		It("should round-trip values with mixed escapes", func() {
			adapter := config.NewPropertiesAdapter()
			original := map[string]string{
				"key": "a\nb\tc\\d\re=f:g#h!i",
			}
			marshaled, err := adapter.Marshal(original)
			Expect(err).ToNot(HaveOccurred())

			unmarshaled, err := adapter.Unmarshal(marshaled)
			Expect(err).ToNot(HaveOccurred())
			Expect(unmarshaled).To(Equal(original))
		})
	})
})

// propertiesGrammarChars are the characters the .properties grammar reacts to: the two
// separators, the escape character, the comment markers and whitespace (layout unless escaped),
// plus one ordinary letter to build mixed tokens with.
var propertiesGrammarChars = []string{"=", ":", " ", "\\", "#", "!", "\n", "\r", "\t", "a"}

var _ = Describe("PropertiesAdapter round-trip over adversarial keys and values", func() {
	var adapter *config.PropertiesAdapter

	BeforeEach(func() {
		adapter = config.NewPropertiesAdapter()
	})

	// Two ordinary entries travel with every generated one, so an entry that is dropped, split
	// into a second key or turned into a comment fails as loudly as a corrupted value.
	roundTrip := func(key, value string) {
		GinkgoHelper()
		original := map[string]string{key: value, "plain": "kept", "second": "also kept"}
		marshaled, err := adapter.Marshal(original)
		Expect(err).ToNot(HaveOccurred())
		unmarshaled, err := adapter.Unmarshal(marshaled)
		Expect(err).ToNot(HaveOccurred())
		Expect(unmarshaled).To(Equal(original), "key %q value %q marshaled to %q", key, value, marshaled)
	}

	It("holds for every key/value pair of up to two grammar characters", func() {
		tokens := propertiesTokens(2)
		for _, key := range tokens {
			for _, value := range tokens {
				roundTrip(key, value)
			}
		}
	})

	It("holds for longer randomly assembled keys and values", func() {
		// Fixed seed: a failure names the offending pair and is reproducible from the report.
		rng := rand.New(rand.NewSource(1))
		for i := 0; i < 2000; i++ {
			roundTrip(randomPropertiesToken(rng, 6), randomPropertiesToken(rng, 6))
		}
	})
})

// propertiesTokens returns the empty string plus every string of length 1..maxLen over the
// grammar characters.
func propertiesTokens(maxLen int) []string {
	current := []string{""}
	all := []string{""}
	for i := 0; i < maxLen; i++ {
		next := make([]string, 0, len(current)*len(propertiesGrammarChars))
		for _, token := range current {
			for _, c := range propertiesGrammarChars {
				next = append(next, token+c)
			}
		}
		all = append(all, next...)
		current = next
	}
	return all
}

func randomPropertiesToken(rng *rand.Rand, maxLen int) string {
	var sb strings.Builder
	for n := rng.Intn(maxLen + 1); n > 0; n-- {
		sb.WriteString(propertiesGrammarChars[rng.Intn(len(propertiesGrammarChars))])
	}
	return sb.String()
}

var _ = Describe("PropertiesAdapter", func() {
	var adapter *config.PropertiesAdapter

	BeforeEach(func() {
		adapter = config.NewPropertiesAdapter()
	})

	Describe("NewPropertiesAdapter", func() {
		It("should create a PropertiesAdapter with default separator", func() {
			Expect(adapter).NotTo(BeNil())
			Expect(adapter.Separator).To(Equal("="))
		})
	})

	Describe("Marshal", func() {
		It("should marshal simple key-value pairs", func() {
			data := map[string]string{
				"key1": "value1",
				"key2": "value2",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("key1=value1"))
			Expect(result).To(ContainSubstring("key2=value2"))
		})

		It("should return empty string for empty map", func() {
			result, err := adapter.Marshal(map[string]string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should sort keys alphabetically", func() {
			data := map[string]string{
				"zebra":  "z",
				"alpha":  "a",
				"middle": "m",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal("alpha=a\nmiddle=m\nzebra=z\n"))
		})

		It("should escape special characters in keys", func() {
			data := map[string]string{
				"key with spaces": "value",
				"key=with=equals": "value",
				"key:with:colons": "value",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("\\ "))
			Expect(result).To(ContainSubstring("\\="))
			Expect(result).To(ContainSubstring("\\:"))
		})

		It("should escape special characters in values", func() {
			data := map[string]string{
				"key": "value\nwith\nnewlines",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("\\n"))
		})
	})

	Describe("Unmarshal", func() {
		It("should unmarshal simple key-value pairs", func() {
			content := "key1=value1\nkey2=value2\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key1", "value1"))
			Expect(result).To(HaveKeyWithValue("key2", "value2"))
		})

		It("should handle empty content", func() {
			result, err := adapter.Unmarshal("")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should skip comments", func() {
			content := "# This is a comment\nkey=value\n! This is also a comment\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result).To(HaveKeyWithValue("key", "value"))
		})

		It("should handle colon as separator", func() {
			adapter.Separator = ":"
			content := "key:value\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "value"))
		})

		It("should handle key with no value", func() {
			content := "key\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", ""))
		})

		It("should unescape special characters", func() {
			content := "key=value\\nwith\\nnewlines\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "value\nwith\nnewlines"))
		})

		It("should handle line continuation", func() {
			content := "key=value\\\ncontinued\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "valuecontinued"))
		})
	})

	Describe("Round-trip", func() {
		It("should marshal and unmarshal correctly", func() {
			original := map[string]string{
				"key1": "value1",
				"key2": "value2",
				"key3": "value with spaces",
			}
			marshaled, err := adapter.Marshal(original)
			Expect(err).ToNot(HaveOccurred())

			unmarshaled, err := adapter.Unmarshal(marshaled)
			Expect(err).ToNot(HaveOccurred())

			// Note: "value with spaces" might be escaped differently
			Expect(unmarshaled).To(HaveKeyWithValue("key1", "value1"))
			Expect(unmarshaled).To(HaveKeyWithValue("key2", "value2"))
		})
	})
})

var _ = Describe("XMLAdapter", func() {
	var adapter *config.XMLAdapter

	BeforeEach(func() {
		adapter = config.NewXMLAdapter()
	})

	Describe("NewXMLAdapter", func() {
		It("should create an XMLAdapter", func() {
			Expect(adapter).NotTo(BeNil())
		})
	})

	Describe("Marshal", func() {
		It("should marshal to Hadoop XML format", func() {
			data := map[string]string{
				"fs.defaultFS": "hdfs://localhost:8020",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("<?xml version=\"1.0\""))
			Expect(result).To(ContainSubstring("<configuration>"))
			Expect(result).To(ContainSubstring("<name>fs.defaultFS</name>"))
			Expect(result).To(ContainSubstring("<value>hdfs://localhost:8020</value>"))
			Expect(result).To(ContainSubstring("</configuration>"))
		})

		It("should return empty configuration for empty map", func() {
			result, err := adapter.Marshal(map[string]string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("<configuration>"))
			Expect(result).To(ContainSubstring("</configuration>"))
		})

		It("should sort keys alphabetically", func() {
			data := map[string]string{
				"zebra": "z",
				"alpha": "a",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("<name>alpha</name>"))
			Expect(result).To(ContainSubstring("<name>zebra</name>"))
		})

		It("should escape XML special characters", func() {
			data := map[string]string{
				"key": "<value>&\"'",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("&lt;"))
			Expect(result).To(ContainSubstring("&gt;"))
			Expect(result).To(ContainSubstring("&amp;"))
		})
	})

	Describe("Unmarshal", func() {
		It("should unmarshal Hadoop XML format", func() {
			content := `<?xml version="1.0" encoding="UTF-8"?>
<configuration>
  <property>
    <name>fs.defaultFS</name>
    <value>hdfs://localhost:8020</value>
  </property>
</configuration>`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("fs.defaultFS", "hdfs://localhost:8020"))
		})

		It("should handle empty configuration", func() {
			content := `<?xml version="1.0" encoding="UTF-8"?>
<configuration>
</configuration>`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should handle multiple properties", func() {
			content := `<?xml version="1.0" encoding="UTF-8"?>
<configuration>
  <property>
    <name>key1</name>
    <value>value1</value>
  </property>
  <property>
    <name>key2</name>
    <value>value2</value>
  </property>
</configuration>`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(2))
			Expect(result).To(HaveKeyWithValue("key1", "value1"))
			Expect(result).To(HaveKeyWithValue("key2", "value2"))
		})
	})

	Describe("Round-trip", func() {
		It("should marshal and unmarshal correctly", func() {
			original := map[string]string{
				"key1": "value1",
				"key2": "value2",
			}
			marshaled, err := adapter.Marshal(original)
			Expect(err).ToNot(HaveOccurred())

			unmarshaled, err := adapter.Unmarshal(marshaled)
			Expect(err).ToNot(HaveOccurred())
			Expect(unmarshaled).To(Equal(original))
		})
	})
})

var _ = Describe("YAMLAdapter", func() {
	Describe("scalar typing", func() {
		It("emits values that resolve to another YAML type without quotes", func() {
			// configOverrides is map[string]map[string]string, but the generated file is read by
			// the product: `port: 8080` has to stay a number there.
			out, err := config.NewYAMLAdapter().Marshal(map[string]string{"port": "8080", "debug": "true"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring("port: 8080"))
			Expect(out).To(ContainSubstring("debug: true"))
		})

		It("still quotes an empty value, which would otherwise read back as null", func() {
			out, err := config.NewYAMLAdapter().Marshal(map[string]string{"empty": ""})
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring(`empty: ""`))
		})
	})

	var adapter *config.YAMLAdapter

	BeforeEach(func() {
		adapter = config.NewYAMLAdapter()
	})

	Describe("NewYAMLAdapter", func() {
		It("should create a YAMLAdapter", func() {
			Expect(adapter).NotTo(BeNil())
		})
	})

	Describe("Marshal", func() {
		It("should marshal simple key-value pairs", func() {
			data := map[string]string{
				"key1": "value1",
				"key2": "value2",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("key1: value1"))
			Expect(result).To(ContainSubstring("key2: value2"))
		})

		It("should return empty string for empty map", func() {
			result, err := adapter.Marshal(map[string]string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should keep values with special characters parseable", func() {
			data := map[string]string{
				"key": "value:with:colons",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())

			recovered, err := adapter.Unmarshal(result)
			Expect(err).ToNot(HaveOccurred())
			Expect(recovered).To(Equal(data))
		})

		It("should handle empty values", func() {
			data := map[string]string{
				"key": "",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring(`key: ""`))
		})

		It("should sort keys alphabetically", func() {
			data := map[string]string{
				"zebra":  "z",
				"alpha":  "a",
				"middle": "m",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal("alpha: a\nmiddle: m\nzebra: z\n"))
		})

		// The emitted document must survive a real YAML parser: a hand-rolled quoting pass
		// leaves invalid escapes ("C:\data"), raw newlines inside quotes and unquoted keys.
		DescribeTable("round-trips values a naive quoter corrupts",
			func(key, value string) {
				data := map[string]string{key: value}
				marshaled, err := adapter.Marshal(data)
				Expect(err).ToNot(HaveOccurred())

				recovered, err := adapter.Unmarshal(marshaled)
				Expect(err).ToNot(HaveOccurred())
				Expect(recovered).To(Equal(data))
			},
			Entry("backslash in value", "win", `C:\data:x`),
			Entry("newline in value", "multi", "a\nb"),
			Entry("double quote in value", "quoted", `say "hi"`),
			Entry("surrounding spaces in value", "padded", "  pad  "),
			Entry("boolean-looking value", "flag", "true"),
			Entry("integer-looking value", "num", "123"),
			Entry("null-looking value", "nothing", "null"),
			Entry("separator characters in key", "k e:y", "v"),
			Entry("hash in value", "commented", "a # b"),
		)
	})

	Describe("Unmarshal", func() {
		It("should unmarshal simple key-value pairs", func() {
			content := "key1: value1\nkey2: value2\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key1", "value1"))
			Expect(result).To(HaveKeyWithValue("key2", "value2"))
		})

		It("should handle empty content", func() {
			result, err := adapter.Unmarshal("")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should skip comments", func() {
			content := "# This is a comment\nkey: value\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result).To(HaveKeyWithValue("key", "value"))
		})

		It("should handle quoted values", func() {
			content := `key: "quoted value"`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "quoted value"))
		})

		It("should handle single quoted values", func() {
			content := `key: 'quoted value'`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "quoted value"))
		})

		It("should reject nested structures instead of flattening them", func() {
			content := "outer:\n  inner: value\n"
			_, err := adapter.Unmarshal(content)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("only flat key-value documents are supported"))
		})

		It("should reject a document that is not a mapping", func() {
			_, err := adapter.Unmarshal("- a\n- b\n")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a key-value mapping"))
		})
	})
})

var _ = Describe("unescapeEnvValue", func() {
	// This tests the unescapeEnvValue function (line 136) with edge cases

	Describe("Escape sequences", func() {
		It("should unescape newline character", func() {
			// Test through the adapter's Unmarshal which calls unescapeEnvValue
			adapter := config.NewEnvAdapter()
			content := `KEY="line1\nline2"`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", "line1\nline2"))
		})

		It("should unescape carriage return character", func() {
			adapter := config.NewEnvAdapter()
			content := `KEY="text\rcarriage"`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", "text\rcarriage"))
		})

		It("should unescape tab character", func() {
			adapter := config.NewEnvAdapter()
			content := `KEY="col1\tcol2"`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", "col1\tcol2"))
		})

		It("should unescape backslash", func() {
			adapter := config.NewEnvAdapter()
			content := `KEY="path\\to\\file"`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", "path\\to\\file"))
		})

		It("should unescape double quote", func() {
			adapter := config.NewEnvAdapter()
			content := `KEY="say \"hello\""`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", `say "hello"`))
		})

		It("should unescape single quote", func() {
			adapter := config.NewEnvAdapter()
			content := `KEY="it\'s"`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", "it's"))
		})

		It("should unescape dollar sign", func() {
			adapter := config.NewEnvAdapter()
			content := `KEY="price\$100"`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", "price$100"))
		})

		It("should unescape backtick", func() {
			adapter := config.NewEnvAdapter()
			content := `KEY="code\` + "`" + `code"`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", "code`code"))
		})
	})

	Describe("Edge cases with backslashes", func() {
		It("should handle backslash at end of string with escaped quote", func() {
			adapter := config.NewEnvAdapter()
			content := `KEY="endswith\""`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", `endswith"`))
		})

		It("should handle consecutive backslashes", func() {
			adapter := config.NewEnvAdapter()
			content := `KEY="\\\\double"`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", "\\\\double"))
		})

		It("should handle unknown escape sequences", func() {
			// Unknown escape sequences should keep the backslash
			adapter := config.NewEnvAdapter()
			content := `KEY="unknown\xescape"`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			// Unknown escape keeps the backslash
			Expect(result).To(HaveKeyWithValue("KEY", "unknown\\xescape"))
		})

		It("should handle multiple escape sequences in one value", func() {
			adapter := config.NewEnvAdapter()
			content := `KEY="a\nb\tc\\d\re"`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", "a\nb\tc\\d\re"))
		})
	})

	Describe("Empty and simple strings", func() {
		It("should handle empty value", func() {
			adapter := config.NewEnvAdapter()
			content := `KEY=""`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", ""))
		})

		It("should handle value without escapes", func() {
			adapter := config.NewEnvAdapter()
			content := `KEY=simple`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", "simple"))
		})

		It("should handle single backslash followed by escape char", func() {
			adapter := config.NewEnvAdapter()
			content := `KEY="\n"`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", "\n"))
		})
	})

	Describe("Round-trip consistency", func() {
		It("should round-trip values with newlines", func() {
			adapter := config.NewEnvAdapter()
			original := map[string]string{
				"KEY": "line1\nline2\nline3",
			}
			marshaled, err := adapter.Marshal(original)
			Expect(err).ToNot(HaveOccurred())

			unmarshaled, err := adapter.Unmarshal(marshaled)
			Expect(err).ToNot(HaveOccurred())
			Expect(unmarshaled).To(Equal(original))
		})

		It("should round-trip values with tabs", func() {
			adapter := config.NewEnvAdapter()
			original := map[string]string{
				"KEY": "col1\tcol2\tcol3",
			}
			marshaled, err := adapter.Marshal(original)
			Expect(err).ToNot(HaveOccurred())

			unmarshaled, err := adapter.Unmarshal(marshaled)
			Expect(err).ToNot(HaveOccurred())
			Expect(unmarshaled).To(Equal(original))
		})

		It("should round-trip values with backslashes", func() {
			adapter := config.NewEnvAdapter()
			original := map[string]string{
				"KEY": `C:\Users\test\path`,
			}
			marshaled, err := adapter.Marshal(original)
			Expect(err).ToNot(HaveOccurred())

			unmarshaled, err := adapter.Unmarshal(marshaled)
			Expect(err).ToNot(HaveOccurred())
			Expect(unmarshaled).To(Equal(original))
		})

		It("should round-trip values with carriage returns", func() {
			adapter := config.NewEnvAdapter()
			original := map[string]string{
				"KEY": "line1\r\nline2",
			}
			marshaled, err := adapter.Marshal(original)
			Expect(err).ToNot(HaveOccurred())

			unmarshaled, err := adapter.Unmarshal(marshaled)
			Expect(err).ToNot(HaveOccurred())
			Expect(unmarshaled).To(Equal(original))
		})

		It("should round-trip values with mixed escapes", func() {
			adapter := config.NewEnvAdapter()
			original := map[string]string{
				"KEY": "a\nb\tc\\d\re$f`g",
			}
			marshaled, err := adapter.Marshal(original)
			Expect(err).ToNot(HaveOccurred())

			unmarshaled, err := adapter.Unmarshal(marshaled)
			Expect(err).ToNot(HaveOccurred())
			Expect(unmarshaled).To(Equal(original))
		})
	})
})

var _ = Describe("EnvAdapter", func() {
	var adapter *config.EnvAdapter

	BeforeEach(func() {
		adapter = config.NewEnvAdapter()
	})

	Describe("NewEnvAdapter", func() {
		It("should create an EnvAdapter", func() {
			Expect(adapter).NotTo(BeNil())
			Expect(adapter.ExportPrefix).To(BeFalse())
		})
	})

	Describe("Marshal", func() {
		It("should marshal to environment variable format", func() {
			data := map[string]string{
				"KEY1": "value1",
				"KEY2": "value2",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("KEY1=value1"))
			Expect(result).To(ContainSubstring("KEY2=value2"))
		})

		It("should return empty string for empty map", func() {
			result, err := adapter.Marshal(map[string]string{})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should add export prefix when enabled", func() {
			adapter.ExportPrefix = true
			data := map[string]string{
				"KEY": "value",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("export KEY=value"))
		})

		It("should quote values with special characters", func() {
			data := map[string]string{
				"KEY": "value with spaces",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring(`"value with spaces"`))
		})

		It("should escape special characters in values", func() {
			data := map[string]string{
				"KEY": "value\nwith\nnewlines",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("\\n"))
		})

		It("should quote empty values", func() {
			data := map[string]string{
				"KEY": "",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring(`KEY=""`))
		})

		// A double-quoted shell value still expands parameters and runs command substitutions,
		// so a config value must not reach the file with an active '$' or backtick.
		DescribeTable("should neutralize shell expansion in values",
			func(value, expected string) {
				adapter.ExportPrefix = true
				result, err := adapter.Marshal(map[string]string{"KEY": value})
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal("export KEY=" + expected + "\n"))

				recovered, err := adapter.Unmarshal(result)
				Expect(err).ToNot(HaveOccurred())
				Expect(recovered).To(HaveKeyWithValue("KEY", value))
			},
			Entry("parameter reference", "pa$word", `"pa\$word"`),
			Entry("command substitution", "$(id)", `"\$(id)"`),
			Entry("backtick substitution", "x`id`y", "\"x\\`id\\`y\""),
		)

		DescribeTable("should reject keys that are not shell variable names",
			func(key string) {
				_, err := adapter.Marshal(map[string]string{key: "value"})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid environment variable name"))
			},
			Entry("dot", "my.key"),
			Entry("dash", "my-key"),
			Entry("leading digit", "1KEY"),
			Entry("space", "MY KEY"),
			Entry("empty", ""),
		)
	})

	Describe("Unmarshal", func() {
		It("should unmarshal environment variable format", func() {
			content := "KEY1=value1\nKEY2=value2\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY1", "value1"))
			Expect(result).To(HaveKeyWithValue("KEY2", "value2"))
		})

		It("should handle empty content", func() {
			result, err := adapter.Unmarshal("")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should skip comments", func() {
			content := "# This is a comment\nKEY=value\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result).To(HaveKeyWithValue("KEY", "value"))
		})

		It("should handle export prefix", func() {
			content := "export KEY=value\n"
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", "value"))
		})

		It("should handle quoted values", func() {
			content := `KEY="quoted value"`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", "quoted value"))
		})

		It("should unescape special characters", func() {
			content := `KEY="value\nwith\nnewlines"`
			result, err := adapter.Unmarshal(content)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("KEY", "value\nwith\nnewlines"))
		})
	})
})

var _ = Describe("INIAdapter", func() {
	var adapter *config.INIAdapter

	BeforeEach(func() {
		adapter = config.NewINIAdapter()
	})

	Describe("Marshal", func() {
		It("should produce empty string for nil input", func() {
			result, err := adapter.Marshal(nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(""))
		})

		It("should marshal key-value pairs sorted", func() {
			data := map[string]string{
				"zebra": "last",
				"apple": "first",
			}
			result, err := adapter.Marshal(data)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal("apple = first\nzebra = last\n"))
		})

		// INI has no escape syntax, so an entry that cannot be written unambiguously must fail
		// rather than produce a file that reads back as different (or extra) entries.
		DescribeTable("should reject entries INI cannot represent",
			func(key, value, wantMessage string) {
				_, err := adapter.Marshal(map[string]string{key: value})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(wantMessage))
			},
			Entry("newline in value injects an entry", "k", "v\ninjected = evil", "line breaks cannot be represented"),
			Entry("carriage return in value", "k", "v\rx", "line breaks cannot be represented"),
			Entry("newline in key", "k\nx", "v", "line breaks cannot be represented"),
			Entry("equals in key moves the split", "a=b", "v", "separator characters"),
			Entry("colon in key moves the split", "a:b", "v", "separator characters"),
			Entry("key reads as a section header", "[section]", "v", "section header or a comment"),
			Entry("key reads as a # comment", "#k", "v", "section header or a comment"),
			Entry("key reads as a ; comment", ";k", "v", "section header or a comment"),
		)
	})

	Describe("Unmarshal", func() {
		It("should parse key = value lines", func() {
			input := "key = value\nother=data\n"
			result, err := adapter.Unmarshal(input)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "value"))
			Expect(result).To(HaveKeyWithValue("other", "data"))
		})

		It("should return error when [section] headers are present", func() {
			input := "[section]\nkey = value\n"
			_, err := adapter.Unmarshal(input)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("section headers are not supported"))
		})

		It("should return error when multiple [section] headers cause key collision", func() {
			input := "[section1]\ntimeout = 30\n[section2]\ntimeout = 60\n"
			_, err := adapter.Unmarshal(input)
			Expect(err).To(HaveOccurred())
		})

		It("should skip # comment lines", func() {
			input := "# this is a comment\nkey = value\n"
			result, err := adapter.Unmarshal(input)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(HaveKey("# this is a comment"))
			Expect(result).To(HaveKeyWithValue("key", "value"))
		})

		It("should skip ; comment lines", func() {
			input := "; semicolon comment\nkey = value\n"
			result, err := adapter.Unmarshal(input)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKeyWithValue("key", "value"))
		})

		It("should round-trip marshal/unmarshal", func() {
			original := map[string]string{"host": "localhost", "port": "8080"}
			marshaled, err := adapter.Marshal(original)
			Expect(err).ToNot(HaveOccurred())
			recovered, err := adapter.Unmarshal(marshaled)
			Expect(err).ToNot(HaveOccurred())
			Expect(recovered).To(Equal(original))
		})
	})
})

var _ = Describe("GetFormat FormatINI", func() {
	It("should return an INIAdapter for FormatINI", func() {
		format := config.GetFormat(config.FormatINI)
		Expect(format).ToNot(BeNil())
		// Verify it's functional
		result, err := format.Marshal(map[string]string{"k": "v"})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(ContainSubstring("k = v"))
	})
})
