package op

import (
	"reflect"
	"strings"

	"github.com/hilthontt/visper/api/infrastructure/config"
	"github.com/hilthontt/visper/api/infrastructure/storage/drivers"
	"github.com/pkg/errors"
)

type DriverConstructor func() drivers.Driver

var driverMap = map[string]DriverConstructor{}
var driverInfoMap = map[string]drivers.Info{}

func RegisterDriver(driver DriverConstructor) {
	tempDriver := driver()
	tempConfig := tempDriver.Config()
	registerDriverItems(tempConfig, tempDriver.GetAddition())
	driverMap[tempConfig.Name] = driver
}

func GetDriver(name string) (DriverConstructor, error) {
	n, ok := driverMap[name]
	if !ok {
		return nil, errors.Errorf("no driver named: %s", name)
	}
	return n, nil
}

func GetDriverNames() []string {
	var driverNames []string
	for k := range driverInfoMap {
		driverNames = append(driverNames, k)
	}
	return driverNames
}

func GetDriverInfoMap() map[string]drivers.Info {
	return driverInfoMap
}

func registerDriverItems(config drivers.Config, addition drivers.Additional) {
	tAddition := reflect.TypeOf(addition)
	for tAddition.Kind() == reflect.Pointer {
		tAddition = tAddition.Elem()
	}
	mainItems := getMainItems(config)
	additionalItems := getAdditionalItems(tAddition, config.DefaultRoot)
	driverInfoMap[config.Name] = drivers.Info{
		Common:     mainItems,
		Additional: additionalItems,
		Config:     config,
	}
}

func getMainItems(cfg drivers.Config) []drivers.Item {
	items := []drivers.Item{{
		Name:     "mount_path",
		Type:     config.TypeString,
		Required: true,
		Help:     "The path you want to mount to, it is unique and cannot be repeated",
	}, {
		Name: "order",
		Type: config.TypeNumber,
		Help: "use to sort",
	}, {
		Name: "remark",
		Type: config.TypeText,
	}}
	if !cfg.NoCache {
		items = append(items, drivers.Item{
			Name:     "cache_expiration",
			Type:     config.TypeNumber,
			Default:  "30",
			Required: true,
			Help:     "The cache expiration time for this storage",
		})
	}
	if !cfg.OnlyProxy && !cfg.OnlyLocal {
		items = append(items, []drivers.Item{{
			Name: "web_proxy",
			Type: config.TypeBool,
		}, {
			Name:     "webdav_policy",
			Type:     config.TypeSelect,
			Options:  "302_redirect,use_proxy_url,native_proxy",
			Default:  "302_redirect",
			Required: true,
		},
		}...)
		if cfg.ProxyRangeOption {
			item := drivers.Item{
				Name: "proxy_range",
				Type: config.TypeBool,
				Help: "Need to enable proxy",
			}
			if cfg.Name == "139Yun" {
				item.Default = "true"
			}
			items = append(items, item)
		}
	} else {
		items = append(items, drivers.Item{
			Name:     "webdav_policy",
			Type:     config.TypeSelect,
			Default:  "native_proxy",
			Options:  "use_proxy_url,native_proxy",
			Required: true,
		})
	}
	items = append(items, drivers.Item{
		Name: "down_proxy_url",
		Type: config.TypeText,
	})
	items = append(items, drivers.Item{
		Name:    "down_proxy_sign",
		Type:    config.TypeBool,
		Default: "true",
	})
	if cfg.LocalSort {
		items = append(items, []drivers.Item{{
			Name:    "order_by",
			Type:    config.TypeSelect,
			Options: "name,size,modified",
		}, {
			Name:    "order_direction",
			Type:    config.TypeSelect,
			Options: "asc,desc",
		}}...)
	}
	items = append(items, drivers.Item{
		Name:    "extract_folder",
		Type:    config.TypeSelect,
		Options: "front,back",
	})
	items = append(items, drivers.Item{
		Name:     "disable_index",
		Type:     config.TypeBool,
		Default:  "false",
		Required: true,
	})
	items = append(items, drivers.Item{
		Name:     "enable_sign",
		Type:     config.TypeBool,
		Default:  "false",
		Required: true,
	})
	return items
}

func getAdditionalItems(t reflect.Type, defaultRoot string) []drivers.Item {
	var items []drivers.Item

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Type.Kind() == reflect.Struct {
			items = append(items, getAdditionalItems(field.Type, defaultRoot)...)
			continue
		}

		tag := field.Tag
		ignore, ok1 := tag.Lookup("ignore")
		name, ok2 := tag.Lookup("json")
		if (ok1 && ignore == "true") || !ok2 {
			continue
		}

		item := drivers.Item{
			Name:     name,
			Type:     strings.ToLower(field.Type.Name()),
			Default:  tag.Get("default"),
			Options:  tag.Get("options"),
			Required: tag.Get("required") == "true",
			Help:     tag.Get("help"),
		}

		itemType := tag.Get("type")
		if itemType != "" {
			item.Type = itemType
		}

		if item.Name == "root_folder_id" || item.Name == "root_folder_path" {
			if item.Default == "" {
				item.Default = defaultRoot
			}
			item.Required = item.Default != ""
		}

		// set default type to string
		if item.Type == "" {
			item.Type = "string"
		}
		items = append(items, item)
	}

	return items
}
