#!/bin/bash

BASE_URL="http://localhost:8080/api"

echo "========================================"
echo "  换班申请系统 API 测试脚本"
echo "========================================"
echo ""

echo "===== 1. 用户登录测试 ====="
echo ""

echo "员工1 (李小明) 登录:"
EMP1_LOGIN=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"employee1","password":"123456"}')
echo "$EMP1_LOGIN" | python3 -m json.tool
EMP1_TOKEN=$(echo "$EMP1_LOGIN" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")
echo "Token: $EMP1_TOKEN"
echo ""

echo "员工2 (王小红) 登录:"
EMP2_LOGIN=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"employee2","password":"123456"}')
echo "$EMP2_LOGIN" | python3 -m json.tool
EMP2_TOKEN=$(echo "$EMP2_LOGIN" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")
echo "Token: $EMP2_TOKEN"
echo ""

echo "经理登录:"
MANAGER_LOGIN=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"manager","password":"123456"}')
MANAGER_TOKEN=$(echo "$MANAGER_LOGIN" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")
echo "经理Token获取成功"
echo ""

echo "===== 2. 班次查询测试 ====="
echo ""

echo "员工1查询自己的班次:"
curl -s -X GET "$BASE_URL/shifts/my" \
  -H "Authorization: Bearer $EMP1_TOKEN" | python3 -m json.tool
echo ""

echo "员工2查询自己的班次:"
curl -s -X GET "$BASE_URL/shifts/my" \
  -H "Authorization: Bearer $EMP2_TOKEN" | python3 -m json.tool
echo ""

echo "===== 3. 发起换班申请 ====="
echo ""

echo "获取员工1的班次ID:"
EMP1_SHIFTS=$(curl -s -X GET "$BASE_URL/shifts/my" \
  -H "Authorization: Bearer $EMP1_TOKEN")
EMP1_SHIFT_ID=$(echo "$EMP1_SHIFTS" | python3 -c "import sys,json; data=json.load(sys.stdin); shifts=data['data']; print(shifts[0]['id']) if shifts else print('1')")
echo "员工1班次ID: $EMP1_SHIFT_ID"
echo ""

echo "获取员工2的班次ID:"
EMP2_SHIFTS=$(curl -s -X GET "$BASE_URL/shifts/my" \
  -H "Authorization: Bearer $EMP2_TOKEN")
EMP2_SHIFT_ID=$(echo "$EMP2_SHIFTS" | python3 -c "import sys,json; data=json.load(sys.stdin); shifts=data['data']; print(shifts[0]['id']) if shifts else print('2')")
echo "员工2班次ID: $EMP2_SHIFT_ID"
echo ""

echo "员工1发起换班申请:"
curl -s -X POST "$BASE_URL/swaps" \
  -H "Authorization: Bearer $EMP1_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"target_user_id\":4,\"requester_shift_id\":$EMP1_SHIFT_ID,\"target_shift_id\":$EMP2_SHIFT_ID,\"reason\":\"家里有事，需要和王小红换班\"}" | python3 -m json.tool
echo ""

echo "===== 4. 查询换班申请列表 ====="
echo ""

echo "员工1查看自己发起的申请:"
curl -s -X GET "$BASE_URL/swaps?as_requester=true" \
  -H "Authorization: Bearer $EMP1_TOKEN" | python3 -m json.tool
echo ""

echo "员工2查看待自己处理的申请:"
curl -s -X GET "$BASE_URL/swaps?as_target=true" \
  -H "Authorization: Bearer $EMP2_TOKEN" | python3 -m json.tool
echo ""

echo "===== 5. 员工2确认换班申请 ====="
echo ""

echo "获取申请ID:"
SWAPS=$(curl -s -X GET "$BASE_URL/swaps?as_target=true" \
  -H "Authorization: Bearer $EMP2_TOKEN")
SWAP_ID=$(echo "$SWAPS" | python3 -c "import sys,json; data=json.load(sys.stdin); swaps=data['data']; print(swaps[0]['id']) if swaps else print('1')")
echo "申请ID: $SWAP_ID"
echo ""

echo "员工2同意换班:"
curl -s -X POST "$BASE_URL/swaps/$SWAP_ID/accept" \
  -H "Authorization: Bearer $EMP2_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"remark":"好的，没问题"}' | python3 -m json.tool
echo ""

echo "===== 6. 经理审批换班申请 ====="
echo ""

echo "经理查询待审批申请:"
curl -s -X GET "$BASE_URL/swaps?status=accepted" \
  -H "Authorization: Bearer $MANAGER_TOKEN" | python3 -m json.tool
echo ""

echo "经理审批通过:"
curl -s -X POST "$BASE_URL/swaps/$SWAP_ID/approve" \
  -H "Authorization: Bearer $MANAGER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"remark":"同意换班"}' | python3 -m json.tool
echo ""

echo "===== 7. 验证班次交换结果 ====="
echo ""

echo "员工1的班次 (应该已经交换):"
curl -s -X GET "$BASE_URL/shifts/my" \
  -H "Authorization: Bearer $EMP1_TOKEN" | python3 -m json.tool
echo ""

echo "员工2的班次 (应该已经交换):"
curl -s -X GET "$BASE_URL/shifts/my" \
  -H "Authorization: Bearer $EMP2_TOKEN" | python3 -m json.tool
echo ""

echo "===== 8. 查询操作日志 ====="
echo ""

echo "员工1查看自己的操作日志:"
curl -s -X GET "$BASE_URL/logs/my" \
  -H "Authorization: Bearer $EMP1_TOKEN" | python3 -m json.tool
echo ""

echo "经理查看所有操作日志:"
curl -s -X GET "$BASE_URL/logs" \
  -H "Authorization: Bearer $MANAGER_TOKEN" | python3 -m json.tool
echo ""

echo "========================================"
echo "  测试完成！"
echo "========================================"
