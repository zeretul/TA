package app

import (
	"strings"
	"ta/internal/interactive"
	"ta/internal/publiquevent"
)

func (a *App) PublicEventStateForLocale(storyID, branchID, locale string) (PublicEventAppState, error) {
	view, err := a.PublicEventStateFor(storyID, branchID)
	return localizePublicEventState(view, locale), err
}

// Translate only visible authored text. No extra stage, participant line or
// permission is introduced by changing language, and stored facts stay intact.
func localizePublicEventState(view PublicEventAppState, locale string) PublicEventAppState {
	if !strings.HasPrefix(locale, "en") {
		return view
	}
	word := func(s string) string {
		if en, ok := tideGaugeEnglish[s]; ok {
			return en
		}
		if en, ok := taiyueEventEnglish[s]; ok {
			return en
		}
		return s
	}
	view.Events = append([]interactive.PublicEventView{}, view.Events...)
	for i := range view.Events {
		e := &view.Events[i]
		if e.EventID != publiquevent.PublicEventIDTideGauge && e.EventID != publiquevent.PublicEventIDTaiyueTide {
			continue
		}
		e.Title, e.Brief, e.Region, e.CurrentStageTitle = word(e.Title), word(e.Brief), word(e.Region), word(e.CurrentStageTitle)
		e.Stages = append([]interactive.PublicEventStageView(nil), e.Stages...)
		for j := range e.Stages {
			s := &e.Stages[j]
			s.Title = word(s.Title)
			s.Lines = append([]string(nil), s.Lines...)
			for k := range s.Lines {
				s.Lines[k] = word(s.Lines[k])
			}
		}
		e.Participants = append([]interactive.PublicEventParticipantView(nil), e.Participants...)
		for j := range e.Participants {
			p := &e.Participants[j]
			p.Name, p.Role = word(p.Name), word(p.Role)
		}
		e.Actions = append([]interactive.PublicEventActionView(nil), e.Actions...)
		for j := range e.Actions {
			a := &e.Actions[j]
			a.Label, a.Brief = word(a.Label), word(a.Brief)
		}
	}
	return view
}

var tideGaugeEnglish = map[string]string{
	"潮信渡的松扣": "The Loose Fastening at Tide Ferry",
	"潮信渡":    "Tide Ferry",
	"宋老橹":    "Old Song",
	"潮信渡到白鹭渔村仍可走十二小时旱路。想随船走，先看看渡头木牌。": "The twelve-hour land route from Tide Ferry to Egret Village remains open. For passage by boat, read the notice at the landing.",
	"老渡公，在渡头贴出当天能走的水路。":               "The old ferryman posts the day's available water routes at the landing.",
	"涨水前": "Before the Rise",
	"涨水后": "After the Rise",
	"渡头木牌写着：下次涨水前，短渡须重新校准。宋老橹站在潮尺旁，正等人来帮忙对照石碑水线。":                  "The landing notice says the short crossing must be calibrated before the next rise. Old Song waits by the tide gauge for help checking it against the stone marker.",
	"你和宋老橹对上石碑水线，压紧滑落的拴扣。他记下短渡水线，答应凭这次校准带你走十小时短渡，涨水以后也认这道记号。":      "You and Old Song align the stone marker's waterline and tighten the slipped fastening. He records the calibrated line and agrees to the ten-hour short crossing; that calibration remains valid after the water rises.",
	"渡头换了木牌：短渡只走涨水前校过的水线。错过校准的客人可找宋老橹复核外汊，或走原来的旱路。木牌上没有写潮尺出了什么问题。": "A new notice reserves the short crossing for waterlines calibrated before the rise. Late arrivals can ask Old Song to check the outer channel or take the original land route. The notice does not explain what happened to the tide gauge.",
	"宋老橹指给你看滑落的拴扣。旧刻痕已没在水下，你们到外桩复核水线，只能确认十一小时的外汊水路。":               "Old Song shows you the slipped fastening. The old marks are submerged, so you check the outer posts together and confirm only the eleven-hour outer-channel route.",
	"找宋老橹校准潮尺": "Calibrate the tide gauge with Old Song",
	"对照石碑与潮尺，确认十小时短渡。涨水前校准后，涨水前后都能出发。": "Compare the stone marker and tide gauge to confirm the ten-hour crossing. Once calibrated before the rise, you may depart before or after the water rises.",
	"找宋老橹复核外汊": "Check the outer channel with Old Song",
	"错过了短渡校准，复核后可走十一小时外汊水路。": "If you missed the short-crossing calibration, checking the outer channel opens an eleven-hour water route.",
}
