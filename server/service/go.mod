module service

go 1.21

require common v0.0.0 // 版本可以是你实际的版本

require (
	github.com/go-redis/redis v6.15.9+incompatible
	gorm.io/gorm v1.23.8
)

require (
	github.com/BurntSushi/toml v1.3.2 // indirect
	github.com/alex-ant/gomath v0.0.0-20160516115720-89013a210a82 // indirect
	github.com/aliyun/aliyun-oss-go-sdk v3.0.2+incompatible // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/gammazero/toposort v0.1.1 // indirect
	github.com/go-sql-driver/mysql v1.6.0 // indirect
	github.com/gofrs/flock v0.8.1 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/magiconair/properties v1.8.6 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pelletier/go-toml/v2 v2.2.2 // indirect
	github.com/qiniu/go-sdk/v7 v7.25.4 // indirect
	github.com/spf13/afero v1.9.2 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spf13/viper v1.12.0 // indirect
	github.com/subosito/gotenv v1.3.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gorm.io/driver/mysql v1.3.6 // indirect
	modernc.org/fileutil v1.0.0 // indirect
)

replace common => ../common
