package utils

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEnv(t *testing.T) {
	t.Run("returns value when env var is set", func(t *testing.T) {
		key := "TEST_GET_ENV_VAR"
		expectedValue := "test_value_123"

		// Set the environment variable
		os.Setenv(key, expectedValue)
		defer os.Unsetenv(key) // Clean up

		result := GetEnv(key)
		assert.Equal(t, expectedValue, result)
	})

	t.Run("returns empty string when env var is not set", func(t *testing.T) {
		key := "TEST_GET_ENV_NONEXISTENT"

		// Make sure it's not set
		os.Unsetenv(key)
		defer os.Unsetenv(key) // Clean up

		result := GetEnv(key)
		assert.Equal(t, "", result)
	})

	t.Run("returns empty string when env var is set to empty", func(t *testing.T) {
		key := "TEST_GET_ENV_EMPTY"

		// Set to empty string
		os.Setenv(key, "")
		defer os.Unsetenv(key) // Clean up

		result := GetEnv(key)
		assert.Equal(t, "", result)
	})

	t.Run("handles special characters in value", func(t *testing.T) {
		key := "TEST_GET_ENV_SPECIAL"
		expectedValue := "value with spaces and @#$% symbols"

		os.Setenv(key, expectedValue)
		defer os.Unsetenv(key)

		result := GetEnv(key)
		assert.Equal(t, expectedValue, result)
	})

	t.Run("handles multiline values", func(t *testing.T) {
		key := "TEST_GET_ENV_MULTILINE"
		expectedValue := "line1\nline2\nline3"

		os.Setenv(key, expectedValue)
		defer os.Unsetenv(key)

		result := GetEnv(key)
		assert.Equal(t, expectedValue, result)
	})
}

func TestGetEnvWithDefault(t *testing.T) {
	t.Run("returns env var value when set", func(t *testing.T) {
		key := "TEST_GET_ENV_WITH_DEFAULT_SET"
		envValue := "env_value"
		defaultValue := "default_value"

		os.Setenv(key, envValue)
		defer os.Unsetenv(key)

		result := GetEnvWithDefault(key, defaultValue)
		assert.Equal(t, envValue, result)
		assert.NotEqual(t, defaultValue, result)
	})

	t.Run("returns default value when env var is not set", func(t *testing.T) {
		key := "TEST_GET_ENV_WITH_DEFAULT_UNSET"
		defaultValue := "default_value_123"

		// Make sure it's not set
		os.Unsetenv(key)
		defer os.Unsetenv(key)

		result := GetEnvWithDefault(key, defaultValue)
		assert.Equal(t, defaultValue, result)
	})

	t.Run("returns default value when env var is empty string", func(t *testing.T) {
		key := "TEST_GET_ENV_WITH_DEFAULT_EMPTY"
		defaultValue := "default_value_456"

		// Set to empty string
		os.Setenv(key, "")
		defer os.Unsetenv(key)

		result := GetEnvWithDefault(key, defaultValue)
		assert.Equal(t, defaultValue, result)
	})

	t.Run("returns env var value even if it matches default", func(t *testing.T) {
		key := "TEST_GET_ENV_WITH_DEFAULT_SAME"
		value := "same_value"

		os.Setenv(key, value)
		defer os.Unsetenv(key)

		result := GetEnvWithDefault(key, value)
		assert.Equal(t, value, result)
	})

	t.Run("handles empty default value", func(t *testing.T) {
		key := "TEST_GET_ENV_WITH_DEFAULT_EMPTY_DEFAULT"

		os.Unsetenv(key)
		defer os.Unsetenv(key)

		result := GetEnvWithDefault(key, "")
		assert.Equal(t, "", result)
	})

	t.Run("handles special characters in default value", func(t *testing.T) {
		key := "TEST_GET_ENV_WITH_DEFAULT_SPECIAL"
		defaultValue := "default with @#$% symbols"

		os.Unsetenv(key)
		defer os.Unsetenv(key)

		result := GetEnvWithDefault(key, defaultValue)
		assert.Equal(t, defaultValue, result)
	})
}

func TestSetEnv(t *testing.T) {
	t.Run("sets environment variable", func(t *testing.T) {
		key := "TEST_SET_ENV_VAR"
		value := "test_set_value"

		// Make sure it's not set initially
		os.Unsetenv(key)
		defer os.Unsetenv(key)

		// Verify it's not set
		assert.Equal(t, "", os.Getenv(key))

		// Set it using our function
		SetEnv(key, value)

		// Verify it's set
		assert.Equal(t, value, os.Getenv(key))
		assert.Equal(t, value, GetEnv(key))
	})

	t.Run("overwrites existing environment variable", func(t *testing.T) {
		key := "TEST_SET_ENV_OVERWRITE"
		oldValue := "old_value"
		newValue := "new_value"

		os.Setenv(key, oldValue)
		defer os.Unsetenv(key)

		// Verify old value
		assert.Equal(t, oldValue, os.Getenv(key))

		// Set new value
		SetEnv(key, newValue)

		// Verify new value
		assert.Equal(t, newValue, os.Getenv(key))
		assert.Equal(t, newValue, GetEnv(key))
	})

	t.Run("sets empty string value", func(t *testing.T) {
		key := "TEST_SET_ENV_EMPTY"

		os.Unsetenv(key)
		defer os.Unsetenv(key)

		SetEnv(key, "")

		// Empty string should be set (different from unset)
		val, exists := os.LookupEnv(key)
		assert.True(t, exists)
		assert.Equal(t, "", val)
	})

	t.Run("handles special characters", func(t *testing.T) {
		key := "TEST_SET_ENV_SPECIAL"
		value := "value with @#$% symbols and spaces"

		os.Unsetenv(key)
		defer os.Unsetenv(key)

		SetEnv(key, value)
		assert.Equal(t, value, os.Getenv(key))
	})
}

func TestUnsetEnv(t *testing.T) {
	t.Run("unsets environment variable", func(t *testing.T) {
		key := "TEST_UNSET_ENV_VAR"
		value := "test_unset_value"

		// Set it first
		os.Setenv(key, value)
		defer os.Unsetenv(key) // Clean up in case test fails

		// Verify it's set
		assert.Equal(t, value, os.Getenv(key))

		// Unset it
		UnsetEnv(key)

		// Verify it's unset
		assert.Equal(t, "", os.Getenv(key))
		_, exists := os.LookupEnv(key)
		assert.False(t, exists)
	})

	t.Run("handles unsetting non-existent variable", func(t *testing.T) {
		key := "TEST_UNSET_ENV_NONEXISTENT"

		// Make sure it's not set
		os.Unsetenv(key)

		// UnsetEnv should not panic
		UnsetEnv(key)

		// Verify it's still not set
		assert.Equal(t, "", os.Getenv(key))
		_, exists := os.LookupEnv(key)
		assert.False(t, exists)
	})

	t.Run("unsets variable that was set to empty string", func(t *testing.T) {
		key := "TEST_UNSET_ENV_EMPTY"

		// Set to empty string
		os.Setenv(key, "")
		defer os.Unsetenv(key)

		// Verify it exists but is empty
		val, exists := os.LookupEnv(key)
		assert.True(t, exists)
		assert.Equal(t, "", val)

		// Unset it
		UnsetEnv(key)

		// Verify it's now unset
		_, exists = os.LookupEnv(key)
		assert.False(t, exists)
	})
}

func TestGetEnvWithDefaultIntegration(t *testing.T) {
	t.Run("integration: set, get, unset, get with default", func(t *testing.T) {
		key := "TEST_INTEGRATION_VAR"
		envValue := "env_set_value"
		defaultValue := "default_fallback"

		// Start with unset
		os.Unsetenv(key)
		defer os.Unsetenv(key)

		// Should return default when unset
		result := GetEnvWithDefault(key, defaultValue)
		assert.Equal(t, defaultValue, result)

		// Set the value
		SetEnv(key, envValue)

		// Should return env value
		result = GetEnvWithDefault(key, defaultValue)
		assert.Equal(t, envValue, result)

		// Unset it
		UnsetEnv(key)

		// Should return default again
		result = GetEnvWithDefault(key, defaultValue)
		assert.Equal(t, defaultValue, result)
	})
}
