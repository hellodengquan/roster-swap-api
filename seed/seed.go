package seed

import (
	"fmt"
	"roster-swap-api/models"
	"roster-swap-api/utils"
	"time"

	"gorm.io/gorm"
)

func SeedData(db *gorm.DB) error {
	var count int64
	db.Model(&models.User{}).Count(&count)
	if count > 0 {
		fmt.Println("数据已存在，跳过种子数据初始化")
		return nil
	}

	fmt.Println("开始初始化种子数据...")

	passwordHash, _ := utils.HashPassword("123456")

	users := []models.User{
		{
			Username:   "admin",
			Password:   passwordHash,
			Name:       "系统管理员",
			Email:      "admin@example.com",
			Phone:      "13800000001",
			Role:       models.RoleAdmin,
			Department: "管理部",
		},
		{
			Username:   "manager",
			Password:   passwordHash,
			Name:       "张经理",
			Email:      "manager@example.com",
			Phone:      "13800000002",
			Role:       models.RoleManager,
			Department: "运营部",
		},
		{
			Username:   "employee1",
			Password:   passwordHash,
			Name:       "李小明",
			Email:      "employee1@example.com",
			Phone:      "13800000011",
			Role:       models.RoleEmployee,
			Department: "运营部",
		},
		{
			Username:   "employee2",
			Password:   passwordHash,
			Name:       "王小红",
			Email:      "employee2@example.com",
			Phone:      "13800000012",
			Role:       models.RoleEmployee,
			Department: "运营部",
		},
		{
			Username:   "employee3",
			Password:   passwordHash,
			Name:       "赵小刚",
			Email:      "employee3@example.com",
			Phone:      "13800000013",
			Role:       models.RoleEmployee,
			Department: "运营部",
		},
	}

	for _, user := range users {
		if err := db.Create(&user).Error; err != nil {
			return fmt.Errorf("创建用户失败: %v", err)
		}
	}

	fmt.Println("用户数据创建完成")

	baseDate := time.Now()
	shiftTypes := []string{"早班", "中班", "晚班"}
	timeSlots := []struct{ start, end string }{
		{"08:00", "16:00"},
		{"12:00", "20:00"},
		{"16:00", "24:00"},
	}

	for i := 0; i < 14; i++ {
		shiftDate := baseDate.AddDate(0, 0, i)
		if shiftDate.Weekday() == time.Saturday || shiftDate.Weekday() == time.Sunday {
			continue
		}

		for userIdx := 2; userIdx <= 4; userIdx++ {
			slotIdx := (i + userIdx) % 3
			shift := models.Shift{
				UserID:    uint(userIdx + 1),
				ShiftDate: shiftDate,
				StartTime: timeSlots[slotIdx].start,
				EndTime:   timeSlots[slotIdx].end,
				ShiftType: shiftTypes[slotIdx],
				Status:    models.ShiftStatusActive,
				Location:  "总部办公区",
				Note:      fmt.Sprintf("周%s排班", []string{"一", "二", "三", "四", "五"}[int(shiftDate.Weekday())-1]),
			}
			if err := db.Create(&shift).Error; err != nil {
				return fmt.Errorf("创建班次失败: %v", err)
			}
		}
	}

	fmt.Println("班次数据创建完成")
	fmt.Println("种子数据初始化完成！")
	fmt.Println("")
	fmt.Println("测试账号:")
	fmt.Println("  管理员: admin / 123456")
	fmt.Println("  经理:   manager / 123456")
	fmt.Println("  员工1:  employee1 / 123456 (李小明)")
	fmt.Println("  员工2:  employee2 / 123456 (王小红)")
	fmt.Println("  员工3:  employee3 / 123456 (赵小刚)")

	return nil
}
