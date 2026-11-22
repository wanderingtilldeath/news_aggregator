package digestmail

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/mail"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// ProReminderWindow is how long before the end of a paid period the one reminder mail goes out.
const ProReminderWindow = 3 * 24 * time.Hour

// ProReminders mails each active Pro member whose period ends within the window, once per period (the store
// remembers which period end was reminded). A failed send is left to the next run; a refused account stops the
// step. With dryRun nothing is recorded, so the same members come up again. It returns how many mails went out.
func ProReminders(ctx context.Context, st store.Store, s mail.Sender, webBase string, dryRun bool, logger *slog.Logger) (int, error) {
	members, err := st.ProExpiring(ctx, ProReminderWindow)
	if err != nil {
		return 0, err
	}

	sent := 0
	for _, m := range members {
		if ctx.Err() != nil {
			return sent, ctx.Err()
		}
		if err := s.Send(ctx, mail.ProExpiringMessage(m.Email, webBase, m.Until)); err != nil {
			if errors.Is(err, mail.ErrRefused) || errors.Is(err, mail.ErrRateLimited) {
				return sent, err
			}
			logger.Warn("pro reminder failed; it is tried again on the next run", "err", err)
			continue
		}
		sent++
		if dryRun {
			continue
		}
		if err := st.ProMarkReminded(ctx, m.Email, m.Until); err != nil {
			return sent, err
		}
	}

	return sent, nil
}
