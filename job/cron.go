package job

import (
	"context"
	"eino-demo/storage/postgres"
	"fmt"
	"github.com/robfig/cron/v3"
	"time"
)

func StartCronJob(pgRepo *postgres.ContractRepo) {
	c := cron.New()

	// 每天凌晨 2 点执行
	_, _ = c.AddFunc("0 0 2 * * *", func() {
		ctx := context.Background()
		rows, err := pgRepo.ExpireContracts(ctx, time.Now())
		if err != nil {
			fmt.Println("[Cron] Error:", err)
		} else {
			fmt.Printf("[Cron] 更新了 %d 份过期合同\n", rows)
		}
	})

	c.Start()
}
