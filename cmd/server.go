// server 模式：加载三文件配置、打开五路 logger、连接数据库（失败则 fatal）、挂存储、
// 装配双平面 Gin（client 与 admin 各自独立端口，共享全部基础设施）、Job worker、优雅退出。
// 运行中数据库 Ping 失败时业务 /api 回 404，Watch 持续重连；/health 仍 200，ready=false。
package cmd

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"

	"github.com/Kirizu-Official/KiriVers/frontend"
	"github.com/Kirizu-Official/KiriVers/internal/cache"
	"github.com/Kirizu-Official/KiriVers/internal/config"
	"github.com/Kirizu-Official/KiriVers/internal/controller"
	"github.com/Kirizu-Official/KiriVers/internal/database"
	"github.com/Kirizu-Official/KiriVers/internal/logger"
	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/urlsign"
)

// runServer 启动 HTTP 更新服务端，阻塞直到收到 SIGINT/SIGTERM 完成优雅退出。
func runServer(cfgPath string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		// listen 前的不可恢复配置错误：直接退出（logging spec 的 fatal 语义）。
		fatal(err)
	}

	var closers []io.Closer
	defer func() {
		for i := len(closers) - 1; i >= 0; i-- {
			_ = closers[i].Close()
		}
	}()

	bg := zerolog.New(io.Discard)
	processSystem := mustOpenStream(cfg.System.Log, bg.With().Str(logger.FieldCat, logger.CatSystem), &closers)
	zlog.Logger = processSystem

	cmdLog := processSystem.With().Str(logger.FieldMod, logger.ModCmd).Logger()
	dbLog := processSystem.With().Str(logger.FieldMod, logger.ModDB).Logger()
	storageLog := processSystem.With().Str(logger.FieldMod, logger.ModStorage).Logger()
	cacheLog := processSystem.With().Str(logger.FieldMod, logger.ModCache).Logger()
	jobLog := processSystem.With().Str(logger.FieldMod, logger.ModJob).Logger()
	auditLog := processSystem.With().Str(logger.FieldMod, logger.ModAudit).Logger()
	ginLog := processSystem.With().Str(logger.FieldMod, logger.ModGin).Logger()

	adminSystem := mustOpenStream(cfg.Admin.Log.System, bg.With().
		Str(logger.FieldCat, logger.CatSystem).
		Str(logger.FieldPlane, logger.PlaneAdmin).
		Str(logger.FieldMod, logger.ModHTTP), &closers)
	adminAccess := mustOpenStream(cfg.Admin.Log.Access, bg.With().
		Str(logger.FieldCat, logger.CatAccess).
		Str(logger.FieldPlane, logger.PlaneAdmin), &closers)
	clientSystem := mustOpenStream(cfg.Client.Log.System, bg.With().
		Str(logger.FieldCat, logger.CatSystem).
		Str(logger.FieldPlane, logger.PlaneClient).
		Str(logger.FieldMod, logger.ModHTTP), &closers)
	clientAccess := mustOpenStream(cfg.Client.Log.Access, bg.With().
		Str(logger.FieldCat, logger.CatAccess).
		Str(logger.FieldPlane, logger.PlaneClient), &closers)

	gin.DefaultWriter = logger.NewStdWriter(ginLog)
	gin.DefaultErrorWriter = logger.NewErrorWriter(ginLog)
	gin.SetMode(processGinMode(cfg.Admin.Mode, cfg.Client.Mode))

	db, err := database.Open(cfg.System.Postgres.DSN, dbLog)
	if err != nil {
		dbLog.Error().Err(err).Msg("open database")
		fatal(err)
	}
	defer database.Close(db)
	if err := database.AutoMigrate(db); err != nil {
		dbLog.Error().Err(err).Msg("auto migrate")
	}

	store, err := storage.Open(storage.Options{
		Driver:    cfg.System.Storage.Driver,
		LocalRoot: cfg.System.Storage.Local.Root,
		S3:        toS3Settings(cfg.System.Storage.S3),
	})
	if err != nil {
		storageLog.Error().Err(err).Msg("open storage; continuing without backend")
		store = nil
	} else {
		probeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := storage.EnsureProbe(probeCtx, store); err != nil {
			storageLog.Error().Err(err).Msg("storage ready probe")
		}
		cancel()
	}

	var privateStore storage.Backend
	if store != nil && strings.EqualFold(strings.TrimSpace(cfg.System.Storage.Driver), "s3") {
		if config.PrivateStorageConfigured(cfg.System) {
			priv, perr := storage.Open(storage.Options{Driver: "s3", S3: toS3Settings(cfg.System.Storage.Private)})
			if perr != nil {
				storageLog.Error().Err(perr).Msg("open private storage; GeoIP upload will fail")
			} else {
				privateStore = priv
				probeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := storage.EnsureProbe(probeCtx, privateStore); err != nil {
					storageLog.Error().Err(err).Msg("private storage ready probe")
				}
				cancel()
			}
		} else {
			storageLog.Warn().Msg("storage.private.bucket empty; GeoIP upload will fail and will not write the public bucket")
		}
	} else {
		privateStore = store
	}

	reconnectMinutes := cfg.System.Cache.Redis.ReconnectIntervalMinutes
	if reconnectMinutes < 1 {
		reconnectMinutes = 1
	}
	cacheStore, err := cache.Open(cache.Options{
		Driver:            cfg.System.Cache.Driver,
		RedisAddr:         cfg.System.Cache.Redis.Addr,
		RedisPassword:     cfg.System.Cache.Redis.Password,
		RedisDB:           cfg.System.Cache.Redis.DB,
		ReconnectInterval: time.Duration(reconnectMinutes) * time.Minute,
		Logger:            cacheLog,
	})
	if err != nil {
		cacheLog.Error().Err(err).Msg("open cache")
		fatal(err)
	}
	closers = append(closers, cacheStore)

	// 私有存储短时签名 URL 密钥（§13.7 / C15-1）：未配置时生成临时随机值并
	// 告警——重启即全部已签发 URL 失效，生产必须显式配置 KIRIVERS_URL_SIGNING_SECRET。
	signingSecret := cfg.System.URLSigningSecret
	if signingSecret == "" {
		tmp, err := model.NewDeviceSecret()
		if err != nil {
			cmdLog.Fatal().Err(err).Msg("generate temporary url signing secret")
		}
		signingSecret = tmp
		cmdLog.Warn().Msg("url_signing_secret not configured; generated temporary secret - signed URLs invalid after restart; set KIRIVERS_URL_SIGNING_SECRET")
	}
	signer := urlsign.NewSigner(signingSecret, time.Duration(model.DefaultSignedURLTTLSeconds)*time.Second)

	admins := service.NewAdminService(service.AdminServiceOptions{
		Store:           repository.NewAdminRepo(db),
		TwoFA:           repository.NewAdmin2FARepo(db),
		Cache:           cacheStore,
		SessionIdle:     time.Duration(cfg.System.Security.SessionIdleHours) * time.Hour,
		PendingTTL:      time.Duration(cfg.System.Security.LoginPendingTTLSeconds) * time.Second,
		TOTPMaxAttempts: cfg.System.Security.TOTPMaxAttemptsPerPeriod,
		WebAuthnRPID:    cfg.System.Security.WebAuthnRPID,
		WebAuthnOrigins: cfg.System.Security.WebAuthnOrigins,
	})
	jobRepo := repository.NewJobRepo(db)
	projects := service.NewProjectService(repository.NewProjectRepo(db), store)
	projects.SetJobStore(jobRepo)
	projects.SetCache(cacheStore)
	projects.SetCacheLogger(cacheLog)
	projects.SetLocalRoot(cfg.System.Storage.Local.Root)
	projects.SetStorageDriver(cfg.System.Storage.Driver)
	clusterActive := config.ClusterActive(cfg)
	projects.SetCluster(clusterActive, cfg.System.Cluster.Download)
	nodeRepo := repository.NewNodeRepo(db)
	projects.SetNodeStore(nodeRepo)
	telRepo := repository.NewTelemetryRepo(db)
	projects.SetTelemetryStore(telRepo)
	// Publish webhook 投递记录仓储（C14-3/C14-4，§5.8）。
	projects.SetWebhookStore(repository.NewWebhookRepo(db))
	projects.SetInstallPolicyStore(repository.NewInstallPolicyRuleRepo(db))
	// 遥测服务（C11）：上报落库、按哈希删除、留存清理与连续失败降级读模型。
	telemetrySvc := service.NewTelemetryService(telRepo)
	telemetrySvc.SetCache(cacheStore)
	telemetrySvc.SetCacheLogger(cacheLog)
	// 隐私删除补全（§11.2 / C15-4）：按哈希删除同时清除灰度白名单行。
	telemetrySvc.SetAllowlistDeleter(repository.NewProjectRepo(db))
	// 审计服务（§11.2 / C15-3/C15-5）：管理动作成功后写入，失败不阻塞。
	auditSvc := service.NewAuditService(repository.NewAuditRepo(db), auditLog)
	projRepo := repository.NewProjectRepo(db)
	announceSvc := service.NewAnnouncementService(repository.NewAnnouncementRepo(db), projRepo)
	var mediaReplica storage.Backend
	if store != nil && strings.EqualFold(strings.TrimSpace(cfg.System.Storage.Driver), "s3") {
		rep, rerr := storage.NewLocalFS(cfg.System.Storage.Local.Root)
		if rerr != nil {
			storageLog.Error().Err(rerr).Msg("open media replica local fs")
			fatal(rerr)
		}
		mediaReplica = rep
	}
	mediaSvc := service.NewMediaService(repository.NewProjectMediaRepo(db), store, mediaReplica)
	geoipSvc := service.NewGeoipService(repository.NewGeoipRepo(db), privateStore, cfg.System.Storage.Local.Root)
	projects.SetGeoipLookup(geoipSvc.LookupFn())
	projects.SetDynamicPackMaxBytes(cfg.System.DynamicPack.MaxBytes)
	projects.SetFileListMaxFiles(cfg.System.FileList.MaxFiles)
	projects.SetChangelogLimits(cfg.System.Changelog.DefaultEntries, cfg.System.Changelog.MaxEntries)

	updateOpts := []update.Option{
		update.WithLineDetails(repository.NewUpdateLineDetailRepo(db)),
		update.WithDowngradeSource(telemetrySvc),
		update.WithURLSigner(signer),
		update.WithPackRuntime(projects),
		update.WithDynamicPackMaxBytes(cfg.System.DynamicPack.MaxBytes),
		update.WithFileListMaxFiles(cfg.System.FileList.MaxFiles),
		update.WithChangelogLimits(cfg.System.Changelog.DefaultEntries, cfg.System.Changelog.MaxEntries),
	}
	if clusterActive && cfg.System.Cluster.Download == config.ClusterDownloadS3 {
		pub := toS3Settings(cfg.System.Storage.S3)
		updateOpts = append(updateOpts, update.WithPublicObjectURL(func(slug, sha256, storageKey string) string {
			key := strings.TrimSpace(storageKey)
			if key == "" {
				key = storage.ArtifactObjectKey(slug, sha256)
			}
			return storage.PublicObjectURL(pub, key)
		}))
	}
	if clusterActive && cfg.System.Cluster.Download == config.ClusterDownloadLocal {
		updateOpts = append(updateOpts, update.WithReplicaGate(projects.ReplicaHas))
	}
	updates := update.NewService(update.NewCachedCatalogLoader(repository.NewUpdateCatalogRepo(db), cacheStore), updateOpts...)

	workerCtx, stopWorkersCancel := context.WithCancel(context.Background())
	defer stopWorkersCancel()
	dbWatch := database.Watch(workerCtx, db, database.DefaultReconnectInterval, dbLog)
	worker := service.NewJobWorker(jobRepo, cfg.System.Jobs.Workers, jobLog)
	if cfg.Admin.Enabled {
		projects.RegisterBundleJobHandler(worker)
		projects.RegisterDeltaJobHandler(worker)
		projects.RegisterAutoDeltaJobHandler(worker)
		projects.RegisterWebhookDeliverJobHandler(worker)
	}
	projects.RegisterDynamicPackJobHandler(worker)

	// 有 local.root 就登记节点并心跳，便于管理台标「当前」。
	// worker.SetNodeID 仅集群：Claim 在 NodeID=uuid.Nil 时不加 owner 过滤。
	// 单例必须是该库上唯一的 worker；不要把 ClusterActive=false 接到已有集群库。
	if strings.TrimSpace(cfg.System.Storage.Local.Root) != "" {
		nodeSvc := service.NewNodeService(service.NodeServiceOptions{
			Store:        nodeRepo,
			LocalRoot:    cfg.System.Storage.Local.Root,
			DisplayName:  cfg.System.Node.DisplayName,
			AdminEnabled: cfg.Admin.Enabled,
			Logger:       processSystem.With().Str(logger.FieldMod, "node").Logger(),
		})
		if row, nerr := nodeSvc.Register(context.Background()); nerr != nil {
			cmdLog.Error().Err(nerr).Msg("register node identity")
		} else {
			projects.SetNodeID(row.ID)
			if clusterActive {
				worker.SetNodeID(row.ID)
				nodeSvc.SetOnBeat(projects.SyncMissingReplicas)
			}
			nodeSvc.StartHeartbeat(workerCtx)
		}
	}

	stopWorkers := worker.Start(workerCtx)
	defer stopWorkers()
	geoipSvc.StartPoll(workerCtx)

	// 单进程双监听（09-14-split-dual-server D1）：client 与 admin 两个引擎各自
	// 监听独立端口，服务层/存储/限流器/签名器等基础设施全部共享单例。
	newEngine := func(recoveryLog, accessLog zerolog.Logger, trustedProxies []string) *gin.Engine {
		e := gin.New()
		if err := middleware.ApplyTrustedProxies(e, trustedProxies); err != nil {
			fatal(err)
		}
		e.Use(middleware.Recovery(recoveryLog), middleware.RequestID(), middleware.AccessLog(accessLog))
		return e
	}
	// 全进程共享限流器（C11-5）：check/diff/telemetry 的 handler 内双维度检查
	// 与 integrity/changelog 的 IP 中间件、CI 路由的 Token 中间件共用一个实例。
	limiter := middleware.NewLimiter()
	adminUI := frontend.Dist()
	deps := controller.Deps{
		Admin:         admins,
		Project:       projects,
		Updates:       updates,
		Telemetry:     telemetrySvc,
		Limiter:       limiter,
		Signer:        signer,
		Audit:         auditSvc,
		Announcements: announceSvc,
		Media:         mediaSvc,
		GeoIP:         geoipSvc,
		Ready: func(ctx context.Context) error {
			if privateStore != nil && privateStore != store {
				return service.CheckReady(ctx, db, store, privateStore)
			}
			return service.CheckReady(ctx, db, store)
		},
		DBAvailable:    dbWatch.Available,
		AdminStaticDir: cfg.Admin.StaticDir,
		AdminStaticFS:  adminUI,
		ClientProxyURL: controller.ClientProxyURLFromAddr(cfg.Client.Addr, cfg.Client.TLSCert != "" && cfg.Client.TLSKey != ""),
	}
	clientEngine := newEngine(clientSystem, clientAccess, cfg.Client.TrustedProxies)
	controller.RegisterClient(clientEngine, deps)
	var adminEngine *gin.Engine
	if adminListenEnabled(cfg.Admin.Enabled) {
		adminEngine = newEngine(adminSystem, adminAccess, cfg.Admin.TrustedProxies)
		controller.RegisterAdmin(adminEngine, deps)
		logAdminStatic(adminSystem, cfg.Admin.StaticDir, adminUI)
	} else {
		adminSystem.Info().Msg("admin plane listen skipped (enabled=false)")
	}

	clientCert, clientKey := cfg.Client.TLSCert, cfg.Client.TLSKey
	adminCert, adminKey := cfg.Admin.TLSCert, cfg.Admin.TLSKey
	clientSrv := &http.Server{
		Addr:              cfg.Client.Addr,
		Handler:           clientEngine,
		ReadHeaderTimeout: 10 * time.Second,
	}
	var adminSrv *http.Server
	if adminListenEnabled(cfg.Admin.Enabled) && adminEngine != nil {
		adminSrv = &http.Server{
			Addr:              cfg.Admin.Addr,
			Handler:           adminEngine,
			ReadHeaderTimeout: 10 * time.Second,
		}
	}

	go func() {
		mode := listenMode(clientCert, clientKey)
		clientSystem.Info().Str("addr", cfg.Client.Addr).Str("listen", mode).Msg("server listening")
		if err := startServer(clientSrv, clientCert, clientKey); err != nil && !errors.Is(err, http.ErrServerClosed) {
			clientSystem.Fatal().Err(err).Msg("listen client plane")
		}
	}()
	if adminSrv != nil {
		go func() {
			mode := listenMode(adminCert, adminKey)
			adminSystem.Info().Str("addr", cfg.Admin.Addr).Str("listen", mode).Msg("server listening")
			if err := startServer(adminSrv, adminCert, adminKey); err != nil && !errors.Is(err, http.ErrServerClosed) {
				adminSystem.Fatal().Err(err).Msg("listen admin plane")
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	// 优雅退出覆盖两个监听器：全部关闭（或超时）后才取消 worker。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := clientSrv.Shutdown(ctx); err != nil {
		clientSystem.Error().Err(err).Msg("shutdown client plane")
	}
	if adminSrv != nil {
		if err := adminSrv.Shutdown(ctx); err != nil {
			adminSystem.Error().Err(err).Msg("shutdown admin plane")
		}
	}
	stopWorkersCancel()
}

func mustOpenStream(cfg config.StreamConfig, ctx zerolog.Context, closers *[]io.Closer) zerolog.Logger {
	log, closer, err := logger.Open(cfg, ctx)
	if err != nil {
		fatal(err)
	}
	*closers = append(*closers, closer)
	return log
}

func toS3Settings(cfg config.S3StorageConfig) storage.S3Settings {
	return storage.S3Settings{
		Endpoint:      cfg.Endpoint,
		Region:        cfg.Region,
		Bucket:        cfg.Bucket,
		AccessKey:     cfg.AccessKey,
		SecretKey:     cfg.SecretKey,
		UsePathStyle:  cfg.UsePathStyle,
		PublicBaseURL: cfg.PublicBaseURL,
	}
}

// adminListenEnabled 决定是否启动管理平面 http.Server（D2）。
func adminListenEnabled(enabled bool) bool {
	return enabled
}

// logAdminStatic 启动时判断管理台来源：空 static_dir 关闭 UI；磁盘 index.html 优先；
// 否则用编译期嵌入的 frontend/dist；两者都没有则 Warn（进程仍提供 API）。
// 使用 admin system 流（调用方已带 plane=admin、mod=http）；请求路径会再 Stat 磁盘，
// 因此随后 yarn build 无需重启即可改用磁盘副本。
func logAdminStatic(log zerolog.Logger, staticDir string, embedded fs.FS) {
	staticDir = strings.TrimSpace(staticDir)
	if staticDir == "" {
		log.Warn().Str("static_dir", "").Msg("admin UI disabled (empty static_dir); serving API only")
		return
	}
	index := filepath.Join(staticDir, "index.html")
	info, err := os.Stat(index)
	if err == nil && !info.IsDir() {
		log.Info().Str("static_dir", staticDir).Msg("admin UI static files enabled")
		return
	}
	if fsHasAdminIndex(embedded) {
		log.Info().Str("static_dir", staticDir).Msg("admin UI embedded files enabled")
		return
	}
	log.Warn().Err(err).Str("static_dir", staticDir).Msg("admin UI index.html missing; serving API only")
}

func fsHasAdminIndex(fsys fs.FS) bool {
	if fsys == nil {
		return false
	}
	info, err := fs.Stat(fsys, "index.html")
	return err == nil && !info.IsDir()
}

// processGinMode：任一平面 mode=debug（忽略大小写）则进程为 gin.DebugMode，否则 ReleaseMode。
func processGinMode(adminMode, clientMode string) string {
	if strings.EqualFold(strings.TrimSpace(adminMode), gin.DebugMode) ||
		strings.EqualFold(strings.TrimSpace(clientMode), gin.DebugMode) {
		return gin.DebugMode
	}
	return gin.ReleaseMode
}
