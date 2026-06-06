package greeting

// tenant is one 그리팅 board in the curated list. The display company name
// normally comes from each opening's group.name; company here is only a
// fallback when that is missing.
type tenant struct {
	slug    string
	company string
}

func (t tenant) host() string { return t.slug + ".career.greetinghr.com" }

// curatedTenants is the hand-verified slug list (checked live 2026-06-06:
// each resolves, exposes __NEXT_DATA__ openings, and yields 신입 dev roles).
// 그리팅 has no public tenant directory and slugs aren't guessable, so this
// list is maintained by hand. Tenants that yielded zero 신입 dev for many
// openings were dropped: 무신사 (custom domain musinsacareers.com, ~135
// openings, all 경력), 컬리 (85 openings, IT-ops only), ezcaretech (referral
// pool only). kakaopay is kept for its seasonal 카카오그룹 신입크루 공채.
var curatedTenants = []tenant{
	{slug: "cashwalk12", company: "넛지헬스케어"},   // 캐시워크 — 백엔드/프론트/iOS/안드로이드/Flutter/데이터, biggest yield
	{slug: "estfamily", company: "이스트소프트"},    // ESTgames/ESTsecurity — game client/server, 보안, DevOps
	{slug: "realworld", company: "RLWRLD"},    // AI robotics — AI Research Engineer
	{slug: "supercent", company: "슈퍼센트"},      // data scientist / 데이터 엔지니어
	{slug: "echomarketing", company: "에코마케팅"}, // 백엔드 인턴/신입
	{slug: "kimcaddie", company: "김캐디"},       // 백엔드 (신입~5년, 병특)
	{slug: "blue-dot", company: "블루닷"},        // 반도체 회로 설계 신입
	{slug: "kakaopay", company: "카카오페이"},      // seasonal 신입크루 공채
}
