package main

// User-facing сообщения (единый язык — русский). Технические детали ошибок
// логируются, пользователю не показываются.

// Валидация.
const (
	msgInvalidBody           = "Некорректное тело запроса"
	msgNameRequired          = "Имя обязательно"
	msgPanelFieldsRequired   = "Название, URL и токен обязательны"
	msgClientFieldsRequired  = "email и inboundId обязательны"
	msgInboundRequired       = "inboundId обязателен"
	msgBadExpiryDate         = "Неверный формат expiryDate (ожидается YYYY-MM-DD)"
	msgNameInvalid           = "имя может содержать только буквы, цифры, _ и -"
	msgNoKeysToTest          = "У подписки нет ключей для теста"
	msgVlessPrefix           = "vlessLink должен начинаться с vless://"
	msgBadKeySourceType      = "type должен быть panel или manual"
	msgPanelKSFieldsRequired = "panelId, clientEmail и inboundId обязательны для type=panel"
	msgKSNotFoundPrefix      = "KeySource не найден: "
	msgKSNoKeyToTest         = "у KeySource нет ключа для теста"
	msgManualNoTraffic       = "manual KeySource не имеет трафика"
)

// Not found / conflict / gateway / auth.
const (
	msgKSPanelDeleted     = "панель KeySource не найдена (удалена?)"
	msgSubNotFound        = "подписка %s не найдена"
	msgNameTaken          = "подписка с именем «%s» уже существует"
	msgPanelUnreachable   = "панель недоступна (таймаут 10 с)"
	msgGetKeyFailed       = "не удалось получить ключ"
	msgSyncScriptNotFound = "скрипт синка не найден: "
	msgUnauthorized       = "не авторизован"
	msgAdminRequired      = "требуется admin-токен"
	msgInternal           = "Внутренняя ошибка"
)

// Внутренние ошибки (детали — в логи).
const (
	msgLoadPanelsFailed   = "Не удалось загрузить панели"
	msgCreatePanelFailed  = "Не удалось создать панель"
	msgListClientsFailed  = "Не удалось получить список клиентов"
	msgCreateClientFailed = "Не удалось создать клиента"
	msgGetKeysFailed      = "Не удалось получить ключи"
	msgListInboundsFailed = "Не удалось получить список инбаундов"
	msgAttachFailed       = "Не удалось привязать инбаунд"
	msgDetachFailed       = "Не удалось отвязать инбаунд"
	msgUpdateClientFailed = "Не удалось обновить клиента"
	msgListSubsFailed     = "Не удалось получить список подписок"
	msgLoadSubsFailed     = "Не удалось загрузить подписки"
	msgWriteSubFileFailed = "Не удалось записать файл подписки"
	msgSaveSubFailed      = "Не удалось сохранить подписку"
	msgResolveKeysFailed  = "Не удалось извлечь ключи"
	msgRegenerateFailed   = "Не удалось перегенерировать"
	msgLoadKSFailed       = "Не удалось загрузить источники ключей"
	msgSaveKSFailed       = "Не удалось сохранить источник ключа"
	msgLoadTokensFailed   = "Не удалось загрузить токены"
	msgCreateTokenFailed  = "Не удалось создать токен"
)

// Отчёты генерации (GenerationReportItem.Why).
const (
	msgRptKSNotFound                 = "KeySource не найден"
	msgRptKSNotFoundRemoved          = "KeySource не найден — ключ удалён"
	msgRptManualSeparate             = "manual-ключи добавляются отдельно"
	msgRptManualLegacy               = "manual (legacy)"
	msgRptPanelDeleted               = "панель удалена"
	msgRptPanelDeletedRemoved        = "панель удалена — ключ удалён"
	msgRptNoPanelKS                  = "нет panel KeySource"
	msgRptClientNotFound             = "клиент не найден на панели — пропущен"
	msgRptClientNotFoundNotIncluded  = "клиент не найден на панели — не включён"
	msgRptInboundNotFound            = "инбаунд не найден на панели — пропущен"
	msgRptInboundNotFoundNotIncluded = "инбаунд не найден на панели — не включён"
)

// Derived-статусы KeySource (ks.Error / ks.Status).
const (
	msgKSPanelDeletedDerived    = "панель удалена — ключ не извлечётся"
	msgKSClientNotFoundDerived  = "клиент не найден на панели — ключ не извлечётся"
	msgKSInboundNotFoundDerived = "инбаунд не найден у клиента — ключ не извлечётся"
	msgKSExpired                = "срок действия ключа истёк — не включается при генерации"
)

// lastTest (тесты ключей через демон).
const (
	msgDaemonUnreachablePrefix = "демон тестов недоступен: "
	msgDaemonParseFailed       = "не удалось разобрать ответ демона"
	msgKeyTestFailed           = "ключ не прошёл тест демона"
)
