package middlewares

import (
	"errors"
	"log"
	"reflect"
	"sync"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
)

type DefaultValidator struct {
	once       sync.Once
	validate   *validator.Validate
	translator ut.Translator
}

var _ binding.StructValidator = &DefaultValidator{}

func (v *DefaultValidator) ValidateStruct(obj any) error {
	if kindOfData(obj) == reflect.Struct {
		v.lazyinit()
		if err := v.validate.Struct(obj); err != nil {
			return err
		}
	}
	return nil
}

func (v *DefaultValidator) Engine() any {
	v.lazyinit()
	return v.validate
}

func (v *DefaultValidator) Translator() ut.Translator {
	v.lazyinit()
	return v.translator
}

func (v *DefaultValidator) lazyinit() {
	v.once.Do(func() {
		v.validate = validator.New(validator.WithRequiredStructEnabled())
		v.validate.SetTagName("binding")

		en := en.New()
		uni := ut.New(en, en)

		v.translator, _ = uni.GetTranslator("en")

		logTranslationErr("default", en_translations.RegisterDefaultTranslations(v.validate, v.translator))

		v.registerCustomTranslations()
	})
}

func (v *DefaultValidator) registerCustomTranslations() {
	logTranslationErr("required", v.validate.RegisterTranslation("required", v.translator, func(ut ut.Translator) error {
		return ut.Add("required", "{0} is required", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("required", fe.Field())
		return t
	}))

	logTranslationErr("max", v.validate.RegisterTranslation("max", v.translator, func(ut ut.Translator) error {
		return ut.Add("max", "{0} must be at most {1}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("max", fe.Field(), fe.Param())
		return t
	}))

	logTranslationErr("min", v.validate.RegisterTranslation("min", v.translator, func(ut ut.Translator) error {
		return ut.Add("min", "{0} must be at least {1}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("min", fe.Field(), fe.Param())
		return t
	}))

	logTranslationErr("len", v.validate.RegisterTranslation("len", v.translator, func(ut ut.Translator) error {
		return ut.Add("len", "{0} must be exactly {1} characters", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("len", fe.Field(), fe.Param())
		return t
	}))

	logTranslationErr("email", v.validate.RegisterTranslation("email", v.translator, func(ut ut.Translator) error {
		return ut.Add("email", "{0} must be a valid email address", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("email", fe.Field())
		return t
	}))

	logTranslationErr("url", v.validate.RegisterTranslation("url", v.translator, func(ut ut.Translator) error {
		return ut.Add("url", "{0} must be a valid URL", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("url", fe.Field())
		return t
	}))

	logTranslationErr("numeric", v.validate.RegisterTranslation("numeric", v.translator, func(ut ut.Translator) error {
		return ut.Add("numeric", "{0} must be a valid numeric value", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("numeric", fe.Field())
		return t
	}))

	logTranslationErr("alpha", v.validate.RegisterTranslation("alpha", v.translator, func(ut ut.Translator) error {
		return ut.Add("alpha", "{0} must contain only letters", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("alpha", fe.Field())
		return t
	}))

	logTranslationErr("alphanum", v.validate.RegisterTranslation("alphanum", v.translator, func(ut ut.Translator) error {
		return ut.Add("alphanum", "{0} must contain only letters and numbers", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("alphanum", fe.Field())
		return t
	}))

	logTranslationErr("gt", v.validate.RegisterTranslation("gt", v.translator, func(ut ut.Translator) error {
		return ut.Add("gt", "{0} must be greater than {1}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("gt", fe.Field(), fe.Param())
		return t
	}))

	logTranslationErr("gte", v.validate.RegisterTranslation("gte", v.translator, func(ut ut.Translator) error {
		return ut.Add("gte", "{0} must be greater than or equal to {1}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("gte", fe.Field(), fe.Param())
		return t
	}))

	logTranslationErr("lt", v.validate.RegisterTranslation("lt", v.translator, func(ut ut.Translator) error {
		return ut.Add("lt", "{0} must be less than {1}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("lt", fe.Field(), fe.Param())
		return t
	}))

	logTranslationErr("lte", v.validate.RegisterTranslation("lte", v.translator, func(ut ut.Translator) error {
		return ut.Add("lte", "{0} must be less than or equal to {1}", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("lte", fe.Field(), fe.Param())
		return t
	}))

	logTranslationErr("oneof", v.validate.RegisterTranslation("oneof", v.translator, func(ut ut.Translator) error {
		return ut.Add("oneof", "{0} must be one of [{1}]", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("oneof", fe.Field(), fe.Param())
		return t
	}))

	logTranslationErr("uuid", v.validate.RegisterTranslation("uuid", v.translator, func(ut ut.Translator) error {
		return ut.Add("uuid", "{0} must be a valid UUID", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("uuid", fe.Field())
		return t
	}))

	logTranslationErr("contains", v.validate.RegisterTranslation("contains", v.translator, func(ut ut.Translator) error {
		return ut.Add("contains", "{0} must contain the text '{1}'", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("contains", fe.Field(), fe.Param())
		return t
	}))

	logTranslationErr("excludes", v.validate.RegisterTranslation("excludes", v.translator, func(ut ut.Translator) error {
		return ut.Add("excludes", "{0} cannot contain the text '{1}'", true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("excludes", fe.Field(), fe.Param())
		return t
	}))
}
func TranslateValidationErrors(err error) []string {
	var messages []string

	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		if v, ok := binding.Validator.(*DefaultValidator); ok {
			trans := v.Translator()
			for _, e := range validationErrs {
				messages = append(messages, e.Translate(trans))
			}
		}
	}

	return messages
}

func TranslateValidationError(err error) string {
	messages := TranslateValidationErrors(err)
	if len(messages) > 0 {
		return messages[0]
	}
	return err.Error()
}

func kindOfData(data any) reflect.Kind {
	value := reflect.ValueOf(data)
	valueType := value.Kind()

	if valueType == reflect.Pointer {
		valueType = value.Elem().Kind()
	}

	return valueType
}

// logTranslationErr reports a failed translation registration. Validation
// keeps working without it, the messages are just less friendly.
func logTranslationErr(tag string, err error) {
	if err != nil {
		log.Printf("validator: registering %q translation: %v", tag, err)
	}
}
