import re

with open('internal/api/webapi/admin.go', 'r') as f:
    code = f.read()

# Fix the duplicate imports
# My previous script added:
# "polaris-hermes/internal/service/sync"
# 	"polaris-hermes/internal/domain"
# Let's remove the extra domain import and alias sync.
code = re.sub(r'\"polaris-hermes/internal/service/sync\"', 'modelsync "polaris-hermes/internal/service/sync"', code)
code = re.sub(r'\t\"polaris-hermes/internal/domain\"\n', '', code)
code = re.sub(r'sync\.NewSyncService', 'modelsync.NewSyncService', code)

# Fix the UpsertSysModel logic in SyncModels
bad_block = r'''			sysModel := &domain\.SysModel\{
				ModelID:       m\.ID,
				ProviderID:    p\.ProviderID,
				ActualModelID: m\.ID,
				DisplayName:   m\.ID,
				VersionWeight: weight,
				IsLegacy:      isLegacy,
			\}
			_ = h\.modelRepo\.UpsertSysModel\(ctx, sysModel\)
			
			_ = h\.intentRepo\.SaveSysIntent\(ctx, &domain\.UserModelIntentDict\{
				ModelID:        m\.ID,
				CapabilityTier: tier,
				Source:         "auto_sync",
			\}\)'''

good_block = '''			sysModel := &domain.SysModel{
				ModelID:       m.ID,
				DisplayName:   m.ID,
				VersionWeight: weight,
				IsLegacy:      isLegacy,
				CapabilityTier: tier,
			}
			_ = h.modelRepo.UpsertSysModel(ctx, sysModel)
			
			pm := &domain.SysProviderModel{
				ProviderID:    p.ProviderID,
				ModelID:       m.ID,
				ActualModelID: m.ID,
			}
			_ = h.modelRepo.UpsertSysProviderModel(ctx, pm)'''

code = re.sub(bad_block, good_block, code)

with open('internal/api/webapi/admin.go', 'w') as f:
    f.write(code)
