package cmd

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
	"strconv"

	"github.com/OpenListTeam/OpenList/cmd/flags"
	_ "github.com/OpenListTeam/OpenList/drivers"
	_ "github.com/OpenListTeam/OpenList/internal/archive"
	_ "github.com/OpenListTeam/OpenList/internal/offline_download"
	"github.com/OpenListTeam/OpenList/internal/op"
	"github.com/OpenListTeam/OpenList/internal/bootstrap"
	"github.com/OpenListTeam/OpenList/internal/conf"
	"github.com/OpenListTeam/OpenList/pkg/utils"
	"github.com/OpenListTeam/OpenList/pkg/utils/random"
	"github.com/OpenListTeam/OpenList/server"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/sffxzzp/go-webview2"
)

var RootCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the server at the specified address",
	Long: `Start the server at the specified address
the address is defined in config file`,
	Run: func(cmd *cobra.Command, args []string) {
		Init()
		if conf.Conf.DelayedStart != 0 {
			utils.Log.Infof("delayed start for %d seconds", conf.Conf.DelayedStart)
			time.Sleep(time.Duration(conf.Conf.DelayedStart) * time.Second)
		}
		bootstrap.InitOfflineDownloadTools()
		bootstrap.LoadStorages()
		bootstrap.InitTaskManager()
		if !flags.Debug && !flags.Dev {
			gin.SetMode(gin.ReleaseMode)
		}
		r := gin.New()
		r.Use(gin.LoggerWithWriter(log.StandardLogger().Out), gin.RecoveryWithWriter(log.StandardLogger().Out))
		server.Init(r)
		var httpSrv, httpsSrv, unixSrv *http.Server
		if conf.Conf.Scheme.HttpPort != -1 {
			httpBase := fmt.Sprintf("%s:%d", conf.Conf.Scheme.Address, conf.Conf.Scheme.HttpPort)
			utils.Log.Infof("start HTTP server @ %s", httpBase)
			httpSrv = &http.Server{Addr: httpBase, Handler: r}
			go func() {
				err := httpSrv.ListenAndServe()
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					utils.Log.Fatalf("failed to start http: %s", err.Error())
				}
			}()
		}
		if conf.Conf.Scheme.HttpsPort != -1 {
			httpsBase := fmt.Sprintf("%s:%d", conf.Conf.Scheme.Address, conf.Conf.Scheme.HttpsPort)
			utils.Log.Infof("start HTTPS server @ %s", httpsBase)
			httpsSrv = &http.Server{Addr: httpsBase, Handler: r}
			go func() {
				err := httpsSrv.ListenAndServeTLS(conf.Conf.Scheme.CertFile, conf.Conf.Scheme.KeyFile)
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					utils.Log.Fatalf("failed to start https: %s", err.Error())
				}
			}()
		}
		if conf.Conf.Scheme.UnixFile != "" {
			utils.Log.Infof("start unix server @ %s", conf.Conf.Scheme.UnixFile)
			unixSrv = &http.Server{Handler: r}
			go func() {
				listener, err := net.Listen("unix", conf.Conf.Scheme.UnixFile)
				if err != nil {
					utils.Log.Fatalf("failed to listen unix: %+v", err)
				}
				// set socket file permission
				mode, err := strconv.ParseUint(conf.Conf.Scheme.UnixFilePerm, 8, 32)
				if err != nil {
					utils.Log.Errorf("failed to parse socket file permission: %+v", err)
				} else {
					err = os.Chmod(conf.Conf.Scheme.UnixFile, os.FileMode(mode))
					if err != nil {
						utils.Log.Errorf("failed to chmod socket file: %+v", err)
					}
				}
				err = unixSrv.Serve(listener)
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					utils.Log.Fatalf("failed to start unix: %s", err.Error())
				}
			}()
		}

		pwdFile := "password.txt"
		if _, err := os.Stat(pwdFile); os.IsNotExist(err) {
			admin, _ := op.GetAdmin()
			newPwd := random.String(8)
			setAdminPassword(newPwd)
			os.WriteFile(pwdFile, []byte("username: "+admin.Username+"\npassword: "+newPwd), 0777)
		}

		var prefix string
		var port int
		if conf.Conf.Scheme.HttpPort != -1 {
			prefix = "http"
			port = conf.Conf.Scheme.HttpPort
		}
		if conf.Conf.Scheme.HttpsPort != -1 {
			prefix = "https"
			port = conf.Conf.Scheme.HttpsPort
		}
		w := webview2.NewWithOptions(webview2.WebViewOptions{
			Debug:    false,
			DataPath: "./data",
			WindowOptions: webview2.WindowOptions{
				Title:  "alist",
				Width:  1440,
				Height: 900,
				IconId: 2,
				Center: true,
			},
		})
		defer w.Destroy()
		w.Navigate(fmt.Sprintf("%s://127.0.0.1:%d", prefix, port))
		w.Run()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// let it directly run from explorer.exe
	cobra.MousetrapHelpText = ""
	RootCmd.PersistentFlags().StringVar(&flags.DataDir, "data", "data", "data folder")
	RootCmd.PersistentFlags().BoolVar(&flags.Debug, "debug", false, "start with debug mode")
	RootCmd.PersistentFlags().BoolVar(&flags.NoPrefix, "no-prefix", false, "disable env prefix")
	RootCmd.PersistentFlags().BoolVar(&flags.Dev, "dev", false, "start with dev mode")
	RootCmd.PersistentFlags().BoolVar(&flags.ForceBinDir, "force-bin-dir", false, "Force to use the directory where the binary file is located as data directory")
	RootCmd.PersistentFlags().BoolVar(&flags.LogStd, "log-std", false, "Force to log to std")
}
