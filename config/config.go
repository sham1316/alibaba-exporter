package config

import (
	"encoding/json"
	"flag"
	configParser "github.com/sham1316/configparser"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sync"
)

var config *Config
var once sync.Once
var configPath *string

func init() {
	configPath = flag.String("config", "config.yaml", "Configuration file path")
	flag.Parse()
}

type password string

func (p password) MarshalJSON() ([]byte, error) {
	if 0 == len(p) {
		return []byte(`""`), nil
	} else {
		return []byte(`"XXX"`), nil
	}
}

type Config struct {
	LogLevel    string `default:"info" env:"LOG_LEVEL"`
	Interval    int    `default:"600" env:"INTERVAL"`
	AlibabaConf struct {
		RegionId        string   `default:"my_region" env:"REGION_ID"`
		AccessKeyId     string   `default:"access_key_id" env:"ACCESS_KEY_ID"`
		AccessKeySecret password `default:"access_key_secret" env:"ACCESS_KEY_SECRET"`
	}
	Http struct {
		Port        string `default:"8080" env:"HTTP_PORT"`
		RoutePrefix string `default:"" env:"HTTP_ROUTE_PREFIX"`
	}
}

func GetCfg() *Config {
	once.Do(func() {
		config = loadConfig(configPath)
		initZap(config)
		b, _ := json.Marshal(config)
		zap.S().Debug(string(b))
	})
	return config
}

func initZap(config *Config) *zap.Logger {
	zapCfg := zap.NewDevelopmentConfig()
	zapCfg.DisableStacktrace = true
	logLevel, _ := zapcore.ParseLevel(config.LogLevel)
	zapCfg.Level = zap.NewAtomicLevelAt(logLevel)
	zapLogger, _ := zapCfg.Build()
	zap.ReplaceGlobals(zapLogger)
	return zapLogger
}
func loadConfig(configFile *string) *Config {
	config := Config{}
	_ = configParser.SetValue(&config, "default")
	configYamlFile, _ := ioutil.ReadFile(*configFile)
	_ = configParser.SetValue(&config, "env")
	_ = yaml.Unmarshal(configYamlFile, &config)
	return &config
}
