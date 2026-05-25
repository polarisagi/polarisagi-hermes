package translator

import (
	"context"
	"net/http"

	"polaris-gateway/internal/service/channel"
)

// Translator 是协议大翻译官的插件式黑盒抽象接口。
// 它完全屏蔽了 OpenAI/Google/Anthropic 等千奇百怪的报文体差异，并自行接管上下游的网络通信流。
// 根据您的指示，为了最小化修改极其复杂的核心转换逻辑（尤其是流式和重试），将执行权交给翻译器。
type Translator interface {
	// TranslateAndExecute 接管整个请求的翻译、上游网络请求以及响应的流式返回
	TranslateAndExecute(ctx context.Context, w http.ResponseWriter, r *http.Request, originalBody []byte, channel *channel.ActiveChannel, targetModel string) error
}

// TranslatorFactory 是一个工厂，根据渠道的协议类型分配对应的 Translator 插件
type TranslatorFactory struct {
	translators map[string]Translator
}

func NewTranslatorFactory() *TranslatorFactory {
	return &TranslatorFactory{
		translators: make(map[string]Translator),
	}
}

// Register 注册翻译器
func (f *TranslatorFactory) Register(protocol string, t Translator) {
	f.translators[protocol] = t
}

// GetTranslator 根据目标系统的协议名字（如 "google", "anthropic"），拿到对应的翻译官
func (f *TranslatorFactory) GetTranslator(targetProtocol string) Translator {
	return f.translators[targetProtocol]
}
