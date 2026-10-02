# Python → Go Port Notları

Kaynak: `devsolartools/solartools-gateway` (Python, private) → hedef: bu repo.
Amaç birebir çeviri değil; Python sürümündeki hataları ve eksikleri kapatarak taşımak.
Her adım ayrı commit; her adımın sonunda **"Go'da ne öğrendik / Python karşılığı"** notu var.

Config formatı: SolarTools platformunun `config_full` ile gönderdiği format
(`connectors[].config.master.slaves[]`, `deviceName`, `timeseries`, `functionCode`, …),
yani solartools-gateway'deki `conf/gateway.json` formatı. İleride ThingsBoard uyumu ayrıca ele alınabilir.

## Nerede kaldık / sırada ne var

**Şu an:** 1. adım bitti (`main`'de, CI yeşil). Sıradaki iş: 2. adım — `settings.py`.

**Bekleyen kararlar**

- [ ] İş akışı: her adım ayrı branch + PR + merge mi olsun (CI merge'den önce PR'da çalışır; GitHub Pull Shark / YOLO rozetleri)?
- [ ] Bulgular (aşağıdaki tablo) GitHub issue olarak açılsın, PR'larla `Closes #n` ile kapansın mı?
- [ ] `shell_exec` (SB-387): sabit izin listesiyle mi taşınsın, hiç taşınmasın mı?

**Yapılacaklar (sırayla)**

- [ ] CI: `actions/checkout@v4` ve `setup-go@v5` Node 20 kullanıyor, kullanımdan kalkıyor → sürüm yükselt (küçük ilk PR adayı)
- [ ] Adım 2 — `settings.py` → `settings/` (SB-389 bozuk config burada kapanır)
- [ ] Adım 3 — `mqtt/buffer.py` ile Go buffer'ı hizala
- [ ] Adım 4 — `timeboxed_io.py` + `publisher.py` → ortak MQTT client
- [ ] Adım 5 — Modbus TCP, ardından RTU (platform formatı; prototip `tcp.go` gider)
- [ ] Adım 6 — `config_receiver.py` → `rpc/` (paralel komut, yapılandırılmış ack, request_id dedupe)
- [ ] Adım 7 — `gateway_service.py` → `gateway/`
- [ ] Adım 8 — `main.py` → `cmd/gateway/`; README'yi yeni yapıya göre güncelle, prototip `config/` paketini kaldır
- [ ] Sonra: S7, OPC-UA, RTU bridge, Enerjisa, REST, SNMP, BACnet, Socket, ZeroExport, log_shipper, updater, provisioning
- [x] Sunucudaki notlar aktarıldı (2026-10-02); port'u etkileyenler aşağıdaki tablolara işlendi
- [ ] Adım 4/7 öncesi: sunucu notlarındaki `_mqtt_deadman_supervisor` (180 sn → `os._exit(70)`) yerel Python klonunda yok, yerine `_mqtt_watchdog` var → solartools-gateway'in güncel `main`'i çekilip hangisinin geçerli olduğu kontrol edilecek

## Günlük

**2026-10-01**

- Sunucu (SSH) internetsiz kaldı; çalışma yerel Windows makinesine taşındı. Konuşma geçmişi ve eski notlar sunucuda kaldı.
- Yerel ortam: GitHub CLI ve Go 1.27 kuruldu; `industrial-gateway` ve referans için `solartools-gateway` klonlandı; Jira (Atlassian MCP) bağlandı.
- Jira tarandı: Go port için ayrı issue yok; Python gateway denetiminde bulunan hatalar SB-386…390 olarak açılmış (tabloya işlendi).
- `gateway_service.py` (1394 satır), `config_receiver.py`, `base.py`, `settings.py` tamamen okundu; plan bağımlılık sırasına göre çıkarıldı.
- Adım 1 tamamlandı ve push edildi (8 commit). Ek olarak: 4 dosyada gofmt düzeltmesi, `.gitattributes` (Windows CRLF), CI'a gofmt + vet kontrolü. CI'da `-race` testleri geçti.

**2026-10-02**

- Sunucudaki memory ve kişisel gelişim notları yerel makineye aktarıldı. Gateway kod incelemesi (2026-09-30), mimari ve saha hata notlarından port'u etkileyenler bu dosyaya işlendi (bulgular 15–18, RTU kararı).

## Plan (bağımlılık sırası)

| Faz | Adım | Python | Go | Durum |
|---|---|---|---|---|
| A. Temel | 1 | `connectors/base.py` | `connector/` | ✅ |
| | 2 | `settings.py` | `settings/` | ⏳ |
| B. Veri yolu | 3 | `mqtt/buffer.py` | `mqtt/buffer/` (Python ile hizala) | ⏳ |
| | 4 | `mqtt/timeboxed_io.py`, `mqtt/publisher.py` | `mqtt/` (ortak client) | ⏳ |
| C. Connector | 5 | `connectors/modbus/modbus_connector.py`, `slave.py` | `connector/modbus/` (önce TCP, sonra RTU) | ⏳ |
| D. Kontrol | 6 | `mqtt/config_receiver.py` | `rpc/` | ⏳ |
| | 7 | `core/gateway_service.py` | `gateway/` | ⏳ |
| | 8 | `main.py` | `cmd/gateway/` | ⏳ |
| Sonra | | S7, OPC-UA, RTU bridge, Enerjisa, REST, SNMP, BACnet, Socket, ZeroExport, log_shipper, updater, provisioning | | |

Mevcut `config/` paketi ve `connector/modbus/tcp.go` port öncesi prototip; 5. ve 7.–8. adımlarda
platform formatına göre yeniden yazılacak, o zamana kadar derlenmeye devam ediyorlar.

## Kararlar

- **Notlar repoda tutulur** (bu dosya). Sunucuda kalan konuşma geçmişi gibi kaybolmasın.
- **Sıra Jira'ya göre değil, bağımlılığa göre.** Jira'daki hatalar ilgili dosya taşınırken kapanır.
- `Connector.update_config` taşınmadı: Python'da hiçbir yerden çağrılmıyor (config değişince connector yeniden başlatılıyor).
- Connector state dosyası (`_load_state`/`_save_state`) sadece Enerjisa kullanıyor → Enerjisa taşınırken eklenecek.
- **Commit'ler küçük ama gerçek parçalar**; her commit tek başına derlenir ve testleri geçer (push öncesi her biri ayrı klasörde doğrulanır). Boş/yapay commit yok.
- Go kaynakları her platformda LF (`.gitattributes`); CI gofmt ve `go vet` hatasında kırılır.
- **Modbus RTU tek döngü kalır.** Python'da RTU önce slave başına task'lardı; paylaşılan seri hatta reconnect fırtınası yarattığı için SB-270'te (2026-06-22) ThingsBoard tarzı tek sıralı döngüye geçildi. Go'da da RTU için slave başına goroutine açılmaz; SB-388 bu döngünün içinde slave başına "sıradaki poll zamanı" ile çözülür. TCP'de slave başına goroutine doğru.
- **Taşınmayacak ölü kod:** `connectors/modbus/server.py` (stub), `publisher.publish_device_connect/disconnect` (hiç çağrılmıyor), `ConnectorType.MQTT_SUB` (implementasyonu yok; sabit sadece tip listesinde duruyor).

## Python'da bulunan sorunlar

| # | Yer | Sorun | Go'da | Jira |
|---|---|---|---|---|
| 1 | `slave.py` | TCP client'a poll ve RPC aynı anda erişiyor, kilit yok | Mutex (prototipte var; 5. adımda) | SB-386 |
| 2 | `gateway_service.py` `_on_shell_exec` | sudo şifresi shell string'ine gömülüyor; şifre MQTT mesajıyla da gelebiliyor; platform her komutu root çalıştırabiliyor | Karar bekliyor: izin listesi ya da hiç taşımamak | SB-387 |
| 3 | `modbus_connector.py` RTU | Tüm slave'ler en hızlı slave'in periyodunda poll ediliyor | Slave başına zamanlama (5. adım) | SB-388 |
| 4 | `settings.py` `load_from_local_config` | Bozuk `gateway.json` → her açılışta çökme | 2. adım | SB-389 |
| 5 | `controllers/zero_export.py` | task index ≠ device index, yanlış cihaz loglanıyor | ZeroExport taşınırken | SB-390 |
| 6 | `config_receiver.py` | Komutlar sırayla işleniyor; uzun `config_full`/`opc_browse` (240 sn'ye kadar) motor durdurma komutunu bekletiyor | Komut başına goroutine (6. adım) | — |
| 7 | `config_receiver.py` `_ack` | Ack her zaman `config_applied: true`; hata sadece `result` içinde | Yapılandırılmış `ok`/`error_code` (6. adım) | SB-380 |
| 8 | `config_receiver.py` | `request_id` ile tekrar kontrolü yok (SB-370 "var" diyor ama yok); tekrar gelen yazma komutu iki kez çalışabilir | Son N request_id dedupe (6. adım) | SB-370 |
| 9 | `gateway_service.py` `handle_device_rpc` | Her komutta tüm config taranıyor; aynı isimli iki cihazda ilk bulunan kazanıyor | Config uygulanırken `map[deviceName]Connector`, çakışma = hata (7. adım) | — |
| 10 | `gateway_service.py` `_safe_eval_expression` | `eval` + boş `__builtins__` güvenli değil (`().__class__…` ile kaçış) | Kendi ifade ayrıştırıcımız (7. adım) | — |
| 11 | `gateway_service.py` `_publish_status` | `uptime_s` = `int(time.time())` → uptime değil epoch | 7. adım | — |
| 12 | `base.py` / `settings.py` | `int(os.getenv(...))` hatalı değerde process'i çökertiyor | `internal/env`: uyarı + varsayılan (1. adım) | — |
| 13 | `base.py` `_safe_run` | Bekleme döngüsü stop'u 5 sn'de bir kontrol ediyor (polling) | `select` + `ctx.Done()` anında uyanıyor (1. adım) | — |
| 14 | `connector/modbus/tcp.go` (Go prototip) | `HandleRPC` yazmayı unit `0xFF`'e gönderiyor, hedef slave'e değil | 5. adımda yeniden yazılıyor | — |
| 15 | `gateway_service.py` `_apply_full_config` | Connector stop'u sınırsız bekliyordu; pymodbus 60 sn bloklayınca config_full zaman aşımı → `_config_apply_lock` tutulu kaldığı için sonraki deploy'lar da zincirleme hata (commit 332fda6 ile 30 sn sınır) | `Base.Stop` zaten sınırlı (1. adım); toplam stop süresi 7. adımda | — |
| 16 | `config_receiver.py` | Zaman aşımında `asyncio.shield` handler'ı arka planda çalıştırmaya devam ediyor ama lock'u tutuyor | Handler'lar `context` ile iptal edilebilir olacak (6. adım) | — |
| 17 | `mqtt/publisher.py` (Mayıs 2026'da düzeltildi) | Buffer boşken flush zaman damgası güncellenmiyordu → deadman 180 sn'de process'i öldürüyordu → RS485 hattı resetleniyor → yine veri yok (kısır döngü) | Sağlık sinyali "veri gönderdim" değil "döngü çalıştı" olmalı (4. adım) | — |
| 18 | `settings.py` | Telemetri flush varsayılanı 50 ms (20 Hz SQLite okuma) gömülü donanımda CPU'yu yoruyor | Varsayılan 500 ms–1 sn düşünülecek (2./4. adım) | — |

---

## Adım 1 — `connectors/base.py` → `connector/`

**Dosyalar**

| Go | Python karşılığı |
|---|---|
| `connector/types.go` | `ConnectorType`, `ConnectorStatus`, `DataPoint` |
| `connector/config.go` | `ConnectorConfig.from_dict`; `DeviceNames()` = `handle_device_rpc` içindeki cihaz adı toplama |
| `connector/connector.go` | `Connector(ABC)` → `Connector` arayüzü + `Protocol` (`_connect`/`_run`/`_disconnect`) |
| `connector/base.go` | `start`, `stop`, `_safe_run`, `_emit`, varsayılan `management_probe`/`get_device_connection_states`/`server_side_rpc_handler` |
| `connector/breaker.go` | `_cb_record_success`, `_cb_record_error` |
| `connector/log.go` | `self.log` + `logLevel` override |
| `internal/env/` | `int(os.getenv(...))` / `float(os.getenv(...))` |

**Python'dan farklar (bilinçli)**

- `_connect() -> bool` yerine `Connect(ctx) error`: hata nedeni loglara taşınıyor.
- `Protocol` metodlarında oluşan **panic yakalanıyor** (`guard`): Python'daki `except Exception` karşılığı. Yakalanmasa tek connector tüm gateway'i çökertirdi.
- Circuit breaker beklemesi 5 sn'lik dilimlerle değil, tek seferde bekliyor; stop gelince anında uyanıyor.
- Breaker üstel gecikmesi taşmaya karşı korumalı (500 hata sonrası bile 10 dk).
- `Validate`: boş `id` de reddediliyor (Python sadece key yoksa KeyError veriyordu).

**Bilinen sınır (Python ile aynı):** `Stop` döngünün çıkmasını en fazla 5 sn bekler; çıkmazsa yine de
devam eder. Bu durumda hemen ardından `Start` çağrılırsa eski döngü bir süre daha yaşayabilir.

**Testler:** breaker gecikme dizisi ve taşma, platform config ayrıştırma (`gateway.json.example`'dan),
eksik alan hatası, `enabled` varsayılanı, tüm cihaz adı kaynakları, null/bozuk doküman,
yeniden bağlanma → active, panic sonrası toparlanma, parent context iptali, takılan Disconnect'te
sınırlı Stop, çift Start, dolu kuyrukta Emit'in bloklamaması, varsayılan handler'lar.
Not: `-race` Windows'ta cgo gerektiriyor; CI'da (Ubuntu) çalışıyor.

### Go'da ne öğrendik / Python karşılığı

| Go | Python | Not |
|---|---|---|
| `interface` (`Connector`, `Protocol`) | `ABC` + `@abstractmethod` | Go'da "implements" yazılmaz; metodları olan her tip arayüzü otomatik sağlar. |
| Struct gömme (`type TCP struct{ *connector.Base }`) + `Protocol` hook | Kalıtım + alt sınıfın `_run`'ı override etmesi | Go'da kalıtım ve sanal metod yok. `Base`, alt tipin metodlarını göremez; bu yüzden alt tip kendini `NewBase(..., t)` ile `Protocol` olarak verir. Gömülen tipin metodları dışarıya "terfi" eder; alt tip aynı isimli metod yazarsa (ör. `HandleRPC`) onunki geçerli olur. |
| `go b.safeRun(ctx, done)` | `asyncio.create_task(self._safe_run())` | Goroutine gerçek paralel çalışır; asyncio task tek thread'de sırayla. |
| `context.WithCancel` + `ctx.Done()` | `asyncio.Event` (`_stop_event`) + `task.cancel()` | Context ağaç gibidir: gateway'in context'i iptal olunca tüm connector'lar da iptal olur. |
| `select { case <-ctx.Done(): case <-t.C: }` | `await asyncio.sleep(...)` + `if stop_event.is_set()` | `select` birden fazla kanalı aynı anda bekler, hangisi önce gelirse. |
| `done chan struct{}` + `close(done)` | `task.done()` | Kapatılmış kanaldan okuma hemen döner; `isOpen` bunu `select … default` ile bloklamadan kontrol ediyor. |
| `select { case out <- dp: default: }` | `queue.put_nowait()` + `except QueueFull` | `default` dalı = "bekleyemiyorsan hemen vazgeç". |
| `make(chan error, 1)` (tamponlu) | — | Disconnect zaman aşımına uğrarsa goroutine sonucu yine yazıp çıkabilsin, sonsuza kadar asılı kalmasın (goroutine sızıntısı). |
| `defer` + `recover()` | `try / except Exception` | `panic` Go'da istisna değil, "programcı hatası"dır; `recover` sadece `defer` içinde çalışır. |
| `error` dönüşü, `fmt.Errorf("...: %w", err)` | `raise` / `except` | Hata bir değerdir; `%w` ile sarmalanınca `errors.Is/As` ile içine bakılabilir. |
| `sync.Mutex` (`status` için) | — (gerek yoktu) | asyncio tek thread olduğu için Python'da yarış yoktu; Go'da heartbeat başka goroutine'den `Status()` okuyacak. |
| `Breaker`'da mutex **yok** | — | Sadece connector döngüsü kullanıyor; her şeye kilit koymak gerekmiyor, sahiplik belgelenmeli. |
| `type Type string` + `const` | `class ConnectorType(str, Enum)` | Tipli string; derleyici `Type` ile düz `string`i karıştırmana izin vermez. |
| `json.RawMessage` | `dict` (`config`) | Belgenin bir kısmını sonra, ihtiyacı olan paket ayrıştırsın diye ham bırakmak. |
| `*bool` (`Enabled`) | `data.get("enabled", True)` | Pointer sayesinde "alan yok" (`nil`) ile `false` ayırt edilir. |
| `log/slog` + `.With("connector", name)` | `logging.getLogger(f"connector.{type}.{name}")` | slog yapılandırılmış log: anahtar=değer çiftleri. Seviye filtresi için küçük bir `slog.Handler` sarmalayıcı yazdık. |
| `os.LookupEnv` | `os.getenv` | "Hiç tanımlı değil" ile "boş" ayrımı. |
| Tablo tabanlı testler (`[]struct{...}` + `t.Run`) | `pytest.mark.parametrize` | |
| `atomic.Int32` (testlerde sayaç) | — | Farklı goroutine'lerden kilitsiz, güvenli sayaç. |
| `t.Setenv` | `monkeypatch.setenv` | Test bitince otomatik geri alınır. |
