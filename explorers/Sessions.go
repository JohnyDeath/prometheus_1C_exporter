package explorer

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"time"

	logrusRotate "github.com/LazarenkoA/LogrusRotate"
	"github.com/LazarenkoA/prometheus_1C_exporter/explorers/model"
	"github.com/prometheus/client_golang/prometheus"
)

type ExplorerSessions struct {
	ExplorerCheckSheduleJob
}

func (this *ExplorerSessions) Construct(s model.Isettings, cerror chan error) *ExplorerSessions {
	this.logger = logrusRotate.StandardLogger().WithField("Name", this.GetName())
	this.logger.Debug("Создание объекта")

	this.summary = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       this.GetName(),
			Help:       "Сессии 1С",
		},
		[]string{"host", "base"},
	)

	// dataGetter - типа мок. Инициализируется из тестов
	if this.BaseExplorer.dataGetter == nil {
		this.BaseExplorer.dataGetter = this.getSessions
	}

	this.settings = s
	this.cerror = cerror
	prometheus.MustRegister(this.summary)
	return this
}

func (this *ExplorerSessions) StartExplore() {
	delay := reflect.ValueOf(this.settings.GetProperty(this.GetName(), "timerNotyfy", 10)).Int()
	this.logger.WithField("delay", delay).Debug("Start")

	timerNotyfy := time.Second * time.Duration(delay)
	this.ticker = time.NewTicker(timerNotyfy)
	host, _ := os.Hostname()
	var groupByDB map[string]int

	this.ExplorerCheckSheduleJob.settings = this.settings
	if err := this.fillBaseList(); err != nil {
		// Если была ошибка это не так критично т.к. через час список повторно обновится. Ошибка может быть если RAS не доступен
		this.logger.WithError(err).Warning("Не удалось получить список баз")
	}

FOR:
	for {
		this.Lock()
		func() {
			logrusRotate.StandardLogger().WithField("Name", this.GetName()).Trace("Старт итерации таймера")
			defer this.Unlock()

			ses, _ := this.BaseExplorer.dataGetter()
			if len(ses) == 0 {
				this.summary.Reset()
				return
			}

			groupByDB = map[string]int{}
			for _, item := range ses {
				groupByDB[this.findBaseName(item["infobase"])]++
			}

			this.summary.Reset()
			// с разбивкой по БД
			for k, v := range groupByDB {
				this.summary.WithLabelValues(host, k).Observe(float64(v))
			}
			// общее кол-во по хосту
			// this.summary.WithLabelValues(host, "").Observe(float64(len(ses)))
		}()

		select {
		case <-this.ctx.Done():
			break FOR
		case <-this.ticker.C:
		}
	}
}

func (this *ExplorerSessions) getSessions() (sesData []map[string]string, err error) {
	sesData = []map[string]string{}

	param := []string{}
	if this.settings.RAC_Host() != "" {
		param = append(param, strings.Join(appendParam([]string{this.settings.RAC_Host()}, this.settings.RAC_Port()), ":"))
	}

	param = append(param, "session")
	param = append(param, "list")
	param = this.appendLogPass(param)

	param = append(param, fmt.Sprintf("--cluster=%v", this.GetClusterID()))

	cmdCommand := exec.Command(this.settings.RAC_Path(), param...)
	if result, err := this.run(cmdCommand); err != nil {
		this.logger.WithError(err).Error()
		return []map[string]string{}, err
	} else {
		this.formatMultiResult(result, &sesData)
	}

	return sesData, nil
}

func (this *ExplorerSessions) GetName() string {
	return "Session"
}
