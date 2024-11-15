package properties_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zncdatadev/operator-go/pkg/config/properties"
)

func TestSave(t *testing.T) {
	// Sample data to save into properties file
	data := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	// Create a temporary file to save properties
	file, err := os.CreateTemp("", "test_save.properties")
	assert.NoError(t, err)
	defer func() {
		err := os.Remove(file.Name())
		assert.NoError(t, err)
	}()

	// Load properties from the map
	props := properties.NewProperties()
	props.LoadFromMap(data)

	// Save properties to the file
	err = props.Save(file.Name())
	assert.NoError(t, err)

	// Load properties from the file to validate
	loadedProps := properties.NewProperties()
	err = loadedProps.LoadFromFile(file.Name())
	assert.NoError(t, err)

	// Validate the saved data
	for key, expectedValue := range data {
		value, exists := loadedProps.Get(key)
		assert.True(t, exists)
		assert.Equal(t, expectedValue, value)
	}
}

func TestSaveWithEmptyProperties(t *testing.T) {
	// Create a temporary file to save properties
	file, err := os.CreateTemp("", "test_save_empty.properties")
	assert.NoError(t, err)
	defer func() {
		err := os.Remove(file.Name())
		assert.NoError(t, err)
	}()

	// Create empty properties
	props := properties.NewProperties()

	// Save properties to the file
	err = props.Save(file.Name())
	assert.NoError(t, err)

	// Load properties from the file to validate
	loadedProps := properties.NewProperties()
	err = loadedProps.LoadFromFile(file.Name())
	assert.NoError(t, err)

	// Validate the saved data
	assert.Empty(t, loadedProps.Keys())
}

func TestSaveWithErrors(t *testing.T) {
	// Attempt to save properties to a non-existent directory
	props := properties.NewProperties()
	err := props.Save("/non_existent_directory/non_existent_file.properties")
	assert.Error(t, err)
}

func TestMarshal(t *testing.T) {
	// Sample data to marshal
	data := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	// Load properties from the map
	props := properties.NewProperties()
	props.LoadFromMap(data)

	// Marshal properties to string
	content, err := props.Marshal()
	assert.NoError(t, err)

	// Expected content
	expectedContent := "key1=value1\nkey2=value2\nkey3=value3\n"

	// Validate the marshaled content
	assert.Equal(t, expectedContent, content)
}

func TestMarshalWithEmptyProperties(t *testing.T) {
	// Create empty properties
	props := properties.NewProperties()

	// Marshal properties to string
	content, err := props.Marshal()
	assert.NoError(t, err)

	// Validate the marshaled content
	assert.Empty(t, content)
}

func TestMarshalWithSpecialCharacters(t *testing.T) {
	// Sample data to marshal
	data := map[string]string{
		"key1": "value1",
		"key2": "value2 with spaces",
		"key3": "value3 with = and #",
	}

	// Load properties from the map
	props := properties.NewProperties()
	props.LoadFromMap(data)

	// Marshal properties to string
	content, err := props.Marshal()
	assert.NoError(t, err)

	// Expected content
	expectedContent := "key1=value1\nkey2=value2 with spaces\nkey3=value3 with = and #\n"

	// Validate the marshaled content
	assert.Equal(t, expectedContent, content)
}

func TestAdd(t *testing.T) {
	// Initialize properties
	props := properties.NewProperties()

	// Add new key-value pairs
	props.Add("key1", "value1")
	props.Add("key2", "value2")

	// Validate the added data
	value, exists := props.Get("key1")
	assert.True(t, exists)
	assert.Equal(t, "value1", value)

	value, exists = props.Get("key2")
	assert.True(t, exists)
	assert.Equal(t, "value2", value)

	// Add a key that already exists with a new value
	props.Add("key1", "new_value1")

	// Validate the updated data
	value, exists = props.Get("key1")
	assert.True(t, exists)
	assert.Equal(t, "new_value1", value)
}

func TestDelete(t *testing.T) {
	// Initialize properties with sample data
	data := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	props := properties.NewProperties()
	props.LoadFromMap(data)

	// Delete an existing key
	props.Delete("key2")

	// Validate the key is deleted
	_, exists := props.Get("key2")
	assert.False(t, exists)

	// Validate other keys are still present
	value, exists := props.Get("key1")
	assert.True(t, exists)
	assert.Equal(t, "value1", value)

	value, exists = props.Get("key3")
	assert.True(t, exists)
	assert.Equal(t, "value3", value)

	// Delete a non-existing key
	props.Delete("key4")

	// Validate it does not affect existing keys
	value, exists = props.Get("key1")
	assert.True(t, exists)
	assert.Equal(t, "value1", value)

	value, exists = props.Get("key3")
	assert.True(t, exists)
	assert.Equal(t, "value3", value)
}

func TestOrder(t *testing.T) {
	// Initialize properties with sample data
	data := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	props := properties.NewProperties()
	props.LoadFromMap(data)

	// Validate the order of keys
	assert.Equal(t, []string{"key1", "key2", "key3"}, props.Keys())

	// Add a new key
	props.Add("key4", "value4")

	// Validate the order of keys
	assert.Equal(t, []string{"key1", "key2", "key3", "key4"}, props.Keys())

	// Delete a key
	props.Delete("key2")

	// Validate the order of keys
	assert.Equal(t, []string{"key1", "key3", "key4"}, props.Keys())
}

func TestNewPropertiesFromFile(t *testing.T) {
	// Create a temporary properties file
	file, err := os.CreateTemp("", "test_new_properties_from_file.properties")
	assert.NoError(t, err)
	defer func() {
		err := os.Remove(file.Name())
		assert.NoError(t, err)
	}()

	// Write sample data to the file
	content := `
# Sample properties file
key1=value1
key2 = value2
key3=value3
`
	_, err = file.WriteString(content)
	assert.NoError(t, err)
	err = file.Close()
	assert.NoError(t, err)

	// Create properties from the file
	props, err := properties.NewPropertiesFromFile(file.Name())
	assert.NoError(t, err)

	// Validate the loaded data
	expected := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	for key, expectedValue := range expected {
		value, exists := props.Get(key)
		assert.True(t, exists)
		assert.Equal(t, expectedValue, value)
	}
}

func TestNewPropertiesFromFileWithEmptyFile(t *testing.T) {
	// Create a temporary empty properties file
	file, err := os.CreateTemp("", "test_new_properties_from_file_empty.properties")
	assert.NoError(t, err)
	defer func() {
		err := os.Remove(file.Name())
		assert.NoError(t, err)
	}()

	// Create properties from the empty file
	props, err := properties.NewPropertiesFromFile(file.Name())
	assert.NoError(t, err)

	// Validate the loaded data
	assert.Empty(t, props.Keys())
}

func TestNewPropertiesFromFileWithNonExistentFile(t *testing.T) {
	// Attempt to create properties from a non-existent file
	_, err := properties.NewPropertiesFromFile("non_existent_file.properties")
	assert.Error(t, err)
}

func TestNewPropertiesFromMap(t *testing.T) {
	// Sample data to initialize properties
	data := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	// Create properties from the map
	props := properties.NewPropertiesFromMap(data)

	// Validate the loaded data
	for key, expectedValue := range data {
		value, exists := props.Get(key)
		assert.True(t, exists)
		assert.Equal(t, expectedValue, value)
	}

	// Validate the order of keys
	assert.Equal(t, []string{"key1", "key2", "key3"}, props.Keys())
}
