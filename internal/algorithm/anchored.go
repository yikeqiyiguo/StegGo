package algorithm

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"sort"

	"steggo/internal/vision"
)

// 特征点锚定隐写（anchored）：学术级几何鲁棒方案。
//
// 原理：在载体上检测 FAST 角点作为锚点，每个锚点的 64×64 邻域块内
// 用二级整数 Haar 变换 + QIM 嵌入两段信息：
//   - 同步区（HL1 子带前 616 位）：伪随机同步码 + 片数 + 锚点 ID + CRC8，
//     每个 bit 7 次冗余投票，用于提取端自定位锚点；
//   - 数据区（LH1/HL2/LH2/HH2/HL1 剩余）：载荷分片，1 bit/系数。
//
// 载荷按"每片 3 个锚点副本"重复嵌入，提取端对副本逐位多数投票。
// 因此对以下攻击天然鲁棒：
//   - JPEG/社交重压缩（QIM 间隔 16 + 冗余投票 + 副本投票）
//   - 平移/裁剪（锚点随内容移动，无需坐标对齐）
//   - 90°/180°/270° 旋转（四方向自动检测，整数旋转无插值误差）
//
// 限制（当前版本）：不支持任意角度旋转与任意比例缩放。
// 锚点 = 最强响应 FAST 角点（间距约束），嵌入/提取无需共享密钥文件。

// ErrNoAnchors 特征点锚定定位失败：图像中未找到可用的同步锚点。
// 这类错误表示"该图像未使用锚定算法嵌入（或已严重破坏）"，与密码/载荷
// 解析错误语义不同，扫描提取时应忽略以免覆盖更精确的失败原因。
var ErrNoAnchors = errors.New("anchored 未找到足够锚点")

// anchorJPEGSimQuality POCS 精化阶段模拟的 JPEG 质量。
// 社交平台分享压缩通常 ≥q75，精化在 q75 下验证读回即可覆盖该场景；
// 更强的压缩（q70 以下）由提取端多副本投票兜底。
const anchorJPEGSimQuality = 75

// anchored 特征点锚定算法。
type anchored struct{}

// NewAnchored 创建特征点锚定算法。
func NewAnchored() Algorithm { return &anchored{} }

const (
	// anchorBlock 锚点邻域块尺寸（64×64）。
	anchorBlock = 64
	// anchorMax 单图最大锚点数。512×512 纹理图实际可检 12+ 个合格角点，
	// 取 12（4 片）保持裁剪鲁棒（每片裁剪后仍保留 ≥1 锚点）；增大到 24
	// 会使片数增至 8、裁剪场景每片副本过薄（crop 测试回归），故容量提升
	// 走「提高每片字节数」（anchorDataVotes=1 → 72B/片）而非堆锚点数。
	anchorMax = 12
	// anchorCopies 每片载荷的锚点副本数（冗余）。
	anchorCopies = 3
	// anchorMinDist 锚点间最小间距（像素），避免块重叠。
	anchorMinDist = 72
	// anchorEdge 锚点距图像边缘的最小距离（保证块完整）。
	anchorEdge = 36
	// anchorMinAnchors 最少锚点数（低于此值拒绝嵌入/提取）。
	anchorMinAnchors = 4
	// anchorQuant 锚定 QIM 量化间隔（容差 ±anchorQuant/2）。
	// JPEG 亮度量化表：q90 DC 步长≈6.4、q80≈12.8、q70≈19.2（误差约半步长）。
	// 间隔 16（容差 8）仅抗 q85+；间隔 32（容差 16）可覆盖 q70（误差≈9.6），
	// 像素扰动 ±16 仍处于压缩域 [16,239] 的边界空间内，POCS 可收敛。
	anchorQuant = 32
	// syncPatternBits 同步码位数。
	syncPatternBits = 64
	// syncFieldBits 同步区总位数：同步码 + 片数(8) + ID(8) + CRC16(16) = 96。
	syncFieldBits = 96
	// syncVotes 同步区每 bit 冗余票数。
	// 同步区位于 LL2 低频子带（JPEG 量化步长极小），2 票已足够稳健。
	syncVotes = 2
	// syncTol 同步码允许的汉明距离（容忍 JPEG 压缩后的少量位错误）。
	syncTol = 3
	// syncScanLimit 提取端同步扫描的最大候选点数上限。
	// 纹理极丰富的图像（噪声/高频照片）FAST 可检出数万点；QIM 嵌入会弱化
	// 锚点角点响应，使锚点排在响应降序的数千名开外，按响应截断会导致锚点
	// 漏扫。故上限设为 65536，覆盖常规图像的全部角点；同步读取只计算
	// 二级 LL 子带（computeLL2，免完整 DWT 与块平面分配），全量扫描开销可控。
	syncScanLimit = 65536
	// ll2Size LL2 子带边长（16×16），二级 Haar 分解的低频子带。
	// LL2 每个系数对应原图 4×4 平均，JPEG 量化误差经平均化后极小，
	// 是抗重压缩最稳的位置；同步区占 LL2 前 192 系数（96 bit × 2 票）。
	ll2Size = anchorBlock / 4
	// anchorCenterThresh 桶内系数距桶中心超过该阈值即重新中心化（粗收敛阶段）。
	// POCS 迭代中"桶正确但接近桶边界"的系数会被其他系数的修正经 DWT
	// 传播的像素扰动（±1 像素 × 平均化，系数级约 ±1~2）偶然翻桶，形成
	// 新错误 → 修正 → 更多扰动 → clamp 截断加剧的雪崩发散。每轮把近边界
	// 系数推回桶中心可切断该扩散链。粗收敛阈值取 8（显著大于传播噪声，
	// 保证所有块可收敛）；收敛后再以紧阈值 2 追加"精细中心化"阶段，把
	// 系数推回桶中心，使残留偏心距叠加 JPEG 量化漂移（彩色噪声中频子带
	// 均值 ~6.6）后仍远小于桶半宽 16，压低重压缩位错误率。
	anchorCenterThresh = 8
	// anchorDataVotes 数据区每 bit 冗余票数。
	// =1 时每片 72B（576 系数 → 576 位）。嵌入端的最终 JPEG 鲁棒性校验
	// 保证保留的锚点读回全对（JPEG 编码确定性），单票即可；投票冗余是
	// 为超出模拟范围的退化场景兜底，提高票数会线性吃掉容量（2 票 → 36B）。
	anchorDataVotes = 1
	// anchorDataBits 每锚点有效数据位数：
	// 数据区系数 = LL2 剩余(256-192=64) + HL2(256) + LH2(256) = 576，
	// 每 bit 以 anchorDataVotes 份冗余，有效位数 = 576 / votes。
	// 一级/二级高频及一级中频子带（HH1/HH2/HL1/LH1）在 JPEG 量化下
	// 错误率过高，QIM 大间隔也无法保证提取，故不承载数据。
	anchorDataBits = (ll2Size*ll2Size - syncFieldBits*syncVotes + 2*anchorBlock*anchorBlock/16) / anchorDataVotes
	// anchorDataBytes 每锚点数据区字节数。
	anchorDataBytes = anchorDataBits / 8
)

func (a *anchored) ID() byte     { return IDANCHORED }
func (a *anchored) Name() string { return "anchored" }

// Capacity 返回锚点方案的可嵌入位数：锚点数 → 片数 → 每片数据字节。
func (a *anchored) Capacity(img *image.NRGBA, opt Options) int {
	if img == nil {
		return 0
	}
	opt.fillDefaults()
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	// 与 Embed 保持同一锚点选择域：Embed 的 selectAnchors 基于压缩域
	// Y 平面（plane[i]=compressY(Y)），若这里用未压缩 Y，FAST 角点数可能
	// 不一致，导致 Capacity 高估容量、Embed 按数据分片越界。
	ycbcr := toYCbCr(img)
	gray := make([]byte, w*h)
	for i := range gray {
		gray[i] = clampByte(compressY(ycbcr[i].Y))
	}
	n := countAnchorCandidates(gray, w, h)
	if n < anchorMinAnchors {
		return 0
	}
	c := (n + anchorCopies - 1) / anchorCopies
	return c * anchorDataBytes * 8
}

// Embed 将位流嵌入锚点邻域块。
func (a *anchored) Embed(img *image.NRGBA, bits []byte, opt Options) error {
	opt.fillDefaults()
	if len(opt.Seed) == 0 {
		return errors.New("anchored 需要坐标种子（--password 派生）")
	}
	capBits := a.Capacity(img, opt)
	if len(bits) > capBits {
		return errCapacity(capBits, len(bits))
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()

	ycbcr := toYCbCr(img)
	plane := make([]int, w*h)
	cbs := make([]uint8, w*h)
	crs := make([]uint8, w*h)
	for i := range plane {
		// 与 DWT 一致：Y 先压缩到 [16,239] 再嵌入。压缩域为高频系数修改
		// 预留边界空间，避免边界像素 clamp 截断形成 POCS 不动点；
		// 提取端直接读取写回的压缩域 Y，两端同域（PNG 无损 / JPEG 量化
		// 均在 [16,239] 附近），LL2 低频子带不受 DC 偏移影响。
		plane[i] = compressY(ycbcr[i].Y)
		cbs[i] = ycbcr[i].Cb
		crs[i] = ycbcr[i].Cr
	}
	// JPEG 4:2:0 色度子采样模型：Cb/Cr 在编码端被 2×2 块平均、解码端按
	// 2×2 复制上采样，像素实际读回的是其所在 2×2 块的色度均值（而非本
	// 像素色度）。彩色高饱和载体（如随机噪声）的 Y↔RGB 往返偏差主要来自
	// 该子采样，POCS 验证必须用子采样色度模拟，否则嵌入状态对 JPEG 退化
	// 不鲁棒（PNG 无损场景读回用精确色度，与子采样模型的偏差仅 ±1~2，
	// 远小于 QIM 桶容差，同样成立）。
	subCbs, subCrs := buildSubsampledChroma(cbs, crs, w, h)

	anchors := a.selectAnchors(plane, w, h)
	n := len(anchors)
	if n < anchorMinAnchors {
		return fmt.Errorf("anchored 特征点不足：仅检测到 %d 个可锚定角点（需要 ≥%d）", n, anchorMinAnchors)
	}
	chunks := (n + anchorCopies - 1) / anchorCopies

	// 位流分片（每片 anchorDataBytes 字节，不足补 0）。
	data := BitsToBytes(bits)
	chunkData := make([][]byte, chunks)
	for c := range chunkData {
		chunkData[c] = make([]byte, anchorDataBytes)
	}
	for i := range data {
		chunkData[i/anchorDataBytes][i%anchorDataBytes] = data[i]
	}

	// 锚点逐一嵌入；POCS 未收敛的锚点跳过（其块已还原为原始内容，
	// 提取端不会误认为锚点）。记录每个锚点嵌入前的块快照，供最终校验
	// 失败时还原（避免错误投票污染片级多数投票）。
	covered := make([]bool, chunks)
	converged := make([]bool, len(anchors))
	preBlock := make([][]int, len(anchors))
	for i, ap := range anchors {
		// 副本按 id 轮询分散到各片（空间均匀），裁剪丢失局部区域时
		// 每个片仍大概率保留副本，避免整片丢失。
		chunkIdx := i % chunks
		preBlock[i] = anchorBlockPixels(plane, w, ap.x, ap.y)
		if embedAnchorBlock(plane, cbs, crs, subCbs, subCrs, w, h, ap.x, ap.y, opt.Seed, i, chunks, chunkData[chunkIdx]) {
			covered[chunkIdx] = true
			converged[i] = true
		}
	}
	for c, ok := range covered {
		if !ok {
			return fmt.Errorf("anchored 锚点覆盖不足: 片 %d 无收敛锚点", c)
		}
	}

	// 最终 JPEG 鲁棒性校验（Gauss-Seidel 循环）：锚点块的 JPEG 读回不仅
	// 取决于块内像素，还取决于 16 对齐扩展区域（DCT/MCU 支持域）内的外部
	// 像素与色度。后嵌入的锚点会改写先嵌入锚点的扩展区域，使精化时验证
	// 通过的状态在最终图像上失效。全部嵌入后按最终环境逐锚点重新验证：
	// 1) 验证通过的保留；
	// 2) 失败的还原到嵌入前块，在当前（接近最终）环境下完整重嵌入
	//    （多起点粗收敛 + JPEG 精化），尽量找到 JPEG 鲁棒状态；
	// 3) 重嵌入仍失败的还原为原始内容并放弃该锚点——保留脏块只会向
	//    片级多数投票注入错误票，移除后剩余副本全部干净。
	for pass := 0; pass < 4; pass++ {
		changed := false
		for i, ap := range anchors {
			if !converged[i] {
				continue
			}
			x0, y0 := ap.x-anchorBlock/2, ap.y-anchorBlock/2
			blk := anchorBlockPixels(plane, w, ap.x, ap.y)
			syncBits := buildSyncBits(opt.Seed, i, chunks)
			dataBits := ByteToBits(chunkData[i%chunks])
			readBack := jpegRoundTripAnchorBlock(blk, plane, cbs, crs, w, h, x0, y0)
			if readBack != nil && verifyAnchorBlock(readBack, syncBits, dataBits) {
				continue
			}
			writeBackBlock(plane, w, ap.x, ap.y, preBlock[i])
			if embedAnchorBlock(plane, cbs, crs, subCbs, subCrs, w, h, ap.x, ap.y, opt.Seed, i, chunks, chunkData[i%chunks]) {
				changed = true
			} else {
				converged[i] = false
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	// 兜底：重嵌入后仍不满足验证的锚点一律还原（移除错误投票）。
	for i, ap := range anchors {
		if !converged[i] {
			continue
		}
		x0, y0 := ap.x-anchorBlock/2, ap.y-anchorBlock/2
		blk := anchorBlockPixels(plane, w, ap.x, ap.y)
		readBack := jpegRoundTripAnchorBlock(blk, plane, cbs, crs, w, h, x0, y0)
		if readBack == nil || !verifyAnchorBlock(readBack, buildSyncBits(opt.Seed, i, chunks), ByteToBits(chunkData[i%chunks])) {
			writeBackBlock(plane, w, ap.x, ap.y, preBlock[i])
			converged[i] = false
		}
	}
	// 还原后重算覆盖：每片仍需 ≥1 个干净锚点。
	for c := range covered {
		covered[c] = false
	}
	for i := range anchors {
		if converged[i] {
			covered[i%chunks] = true
		}
	}
	for c, ok := range covered {
		if !ok {
			return fmt.Errorf("anchored 锚点覆盖不足: 片 %d 无收敛锚点", c)
		}
	}

	for i := range plane {
		ycbcr[i].Y = clampByte(plane[i])
	}
	fromYCbCr(img, ycbcr)
	return nil
}

// Extract 提取位流：四方向扫描同步区定位锚点，副本投票重组载荷。
func (a *anchored) Extract(img *image.NRGBA, opt Options) ([]byte, error) {
	opt.fillDefaults()
	if len(opt.Seed) == 0 {
		return nil, errors.New("anchored 需要坐标种子（--password 派生）")
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	ycbcr := toYCbCr(img)
	gray := make([]byte, w*h)
	for i := range gray {
		gray[i] = ycbcr[i].Y
	}

	// 四方向扫描：旋转图像后找同步区，命中最多者为正确方向。
	// 同步模式门槛极严（2^-58），旋转图像在错误方向几乎必然 0 命中，
	// 故任一方向一旦找到足够锚点即可提前退出，避免无谓扫描。
	var best []syncHit
	var bestGray []byte
	bestW, bestH := 0, 0
	for d := 0; d < 4; d++ {
		g, gw, gh := gray, w, h
		if d > 0 {
			g, gw, gh = vision.Rotate90(gray, w, h, d)
		}
		hits := findSyncHits(g, gw, gh, opt.Seed)
		if len(hits) > len(best) {
			best, bestGray, bestW, bestH = hits, g, gw, gh
		}
		if len(best) >= anchorMinAnchors {
			break
		}
	}
	if len(best) < anchorMinAnchors {
		return nil, fmt.Errorf("%w：图像可能被严重破坏、未使用锚定算法嵌入，或尺寸/方向不支持", ErrNoAnchors)
	}

	// 片数取众数（各锚点同步区冗余携带）。
	chunks := modeChunks(best)
	if chunks <= 0 {
		return nil, fmt.Errorf("%w：同步区损坏，无法读取片数", ErrNoAnchors)
	}

	totalBits := chunks * anchorDataBytes * 8
	outBits := make([]byte, totalBits)

	// 分片副本收集与逐位投票（id 轮询分散，与嵌入端 chunkIdx = id % chunks 一致）。
	copies := make([][][]byte, chunks)
	for _, hit := range best {
		c := hit.id % chunks
		if c >= chunks || c < 0 {
			continue
		}
		copies[c] = append(copies[c], readAnchorData(bestGray, bestW, bestH, hit.x, hit.y))
	}
	for c := 0; c < chunks; c++ {
		chunk := voteChunk(copies[c])
		for i := 0; i < anchorDataBytes; i++ {
			for j := 0; j < 8; j++ {
				outBits[c*anchorDataBytes*8+i*8+j] = (chunk[i] >> uint(7-j)) & 1
			}
		}
	}
	return outBits, nil
}

// ---- 嵌入辅助 ----

// anchorDataRefs 按固定顺序收集数据区全部系数的引用（必须与提取端严格一致）。
// 顺序：LL2 剩余(64) + HL2(256) + LH2(256) = 576。
//   - LL2 是 blk 左上 16×16 区域（二级分解后保留在 blk 中），前 192 系数
//     被同步区占用，其余承载数据；
//   - HL2/LH2 是二级中频子带（更低频，JPEG 量化步长小，抗性最好）；
//   - 一级子带（HL1/LH1/HH1）与 HH2 在 JPEG 量化下错误率过高，不承载数据。
//
// 线性索引 k 对应的数据位为 bits[k%anchorDataBits]（第 k/anchorDataBits 票）。
func anchorDataRefs(blk []int, bands []*dwtBand) []*int {
	refs := make([]*int, 0, anchorDataBits*anchorDataVotes)
	for y := 0; y < ll2Size; y++ {
		for x := 0; x < ll2Size; x++ {
			if y*ll2Size+x < syncFieldBits*syncVotes {
				continue // 同步区
			}
			refs = append(refs, &blk[y*anchorBlock+x])
		}
	}
	for i := range bands[3].coeff {
		refs = append(refs, &bands[3].coeff[i])
	}
	for i := range bands[4].coeff {
		refs = append(refs, &bands[4].coeff[i])
	}
	return refs
}

// writeAnchorData 将数据位以 votes 份冗余写入数据区（全量 QIM，供首轮嵌入）。
// 位 i 的副本位于线性索引 v*len(bits)+i（v=0..votes-1），跨子带分散。
func writeAnchorData(blk []int, bands []*dwtBand, dataBits []byte) {
	refs := anchorDataRefs(blk, bands)
	for i := range dataBits {
		for v := 0; v < anchorDataVotes; v++ {
			k := v*len(dataBits) + i
			*refs[k] = anchQIM(*refs[k], dataBits[i])
		}
	}
}

// fixAnchorData 选择性投影：仅修正桶错误的系数（正确系数保持不动，
// 避免 POCS 全量重嵌把已收敛系数扰动到错误桶）。
func fixAnchorData(blk []int, bands []*dwtBand, dataBits []byte, thresh int) {
	refs := anchorDataRefs(blk, bands)
	for k := 0; k < len(refs); k++ {
		bit := dataBits[k%len(dataBits)]
		// 桶错误或距桶中心过近边界：一律重新中心化（切断翻桶雪崩链）。
		if anchExtract(*refs[k]) != bit || anchDistCenter(*refs[k]) > thresh {
			*refs[k] = anchQIM(*refs[k], bit)
		}
	}
}

// anchDistCenter 返回系数到所在桶中心的距离（0 ~ anchorQuant/2）。
func anchDistCenter(k int) int {
	abs := k
	if k < 0 {
		abs = -k
	}
	rem := abs % anchorQuant
	if rem < anchorQuant/2 {
		return anchorQuant/2 - rem
	}
	return rem - anchorQuant/2
}

// readAnchorDataCoeff 按固定顺序收集数据区系数（只读聚合，与 anchorDataRefs 顺序一致）。
func readAnchorDataCoeff(blk []int, bands []*dwtBand, out []int) {
	idx := 0
	for y := 0; y < ll2Size; y++ {
		for x := 0; x < ll2Size; x++ {
			if y*ll2Size+x < syncFieldBits*syncVotes {
				continue
			}
			out[idx] = blk[y*anchorBlock+x]
			idx++
		}
	}
	for _, c := range bands[3].coeff {
		out[idx] = c
		idx++
	}
	for _, c := range bands[4].coeff {
		out[idx] = c
		idx++
	}
}

// embedAnchorBlock 在单个锚点块内嵌入同步区与数据区（块级 POCS 迭代）。
//
// 采用"选择性投影"POCS：每轮分解 → 仅修正"桶错误"的系数 →
// 逆变换 → clamp + RGB 往返（roundTripY 模拟提取端读取链路）读回验证。
// 正确系数保持不动，避免传统全量重嵌在 QIM 间隔较大时把已收敛系数
// 重新扰动到错误桶（不动点陷阱）；验证通过则写回像素。
//
// 同步区位于 LL2 低频子带（JPEG 量化步长极小，抗重压缩最稳）；
// 数据区覆盖一级/二级中频子带（HL1/LH1/HL2/LH2），每 bit 以
// anchorDataVotes 份冗余，提取端多数投票纠正 JPEG 量化错误。
func embedAnchorBlock(plane []int, cbs, crs, subCbs, subCrs []uint8, w, h, ax, ay int, seed []byte, id, chunks int, chunk []byte) bool {
	blk := anchorBlockPixels(plane, w, ax, ay)
	// 粗收敛阶段的 RGB 往返模拟用 JPEG 4:2:0 子采样色度（建模解码端色度
	// 上采样对 Y↔RGB 往返的影响）；精化阶段用精确色度，因为 JPEG 编码自身
	// 会做子采样（见 jpegRoundTripAnchorBlock）。
	blkCb := anchorBlockBytes(subCbs, w, ax, ay)
	blkCr := anchorBlockBytes(subCrs, w, ax, ay)
	orig := make([]int, len(blk))
	copy(orig, blk)
	syncBits := buildSyncBits(seed, id, chunks)
	dataBits := ByteToBits(chunk)

	// 多起点 POCS：原始块 + 确定性小扰动。
	// 纹理极强块（随机噪声/高频照片）的 DWT 高频系数很大，QIM 修正经逆变换
	// 后像素频繁越界，clamp 截断使修正量丢失，系数桶错误永远无法消除，
	// 交替投影在不相交约束集间振荡发散（同步/数据错误随迭代增长）。
	// 确定性扰动（±1）改变像素相位后可跳出不动点陷阱（与 DCT 方案一致）。
	// 注意：起点数组会被原地分解破坏，必须独立拷贝。
	first := make([]int, len(blk))
	copy(first, blk)
	starts := [][]int{first}
	rng := NewRNG(append(append([]byte{}, seed...), byte(id), byte(chunks)))
	for r := 0; r < 6; r++ {
		s := make([]int, len(blk))
		for i := range s {
			d := 0
			for b := 0; b < 3; b++ {
				d += int(rng.NextBit())
			}
			s[i] = int(clampByte(blk[i] + d - 1))
		}
		starts = append(starts, s)
	}
	// JPEG 精化：roundTripY 模型只覆盖 RGB 往返，真实 JPEG 还有 4:2:0
	// 色度子采样与亮度 DCT 量化（彩色高饱和载体上中频系数漂移可达 ±30），
	// 必须在精化阶段用真实 JPEG q75 编解码模拟把系数推到 JPEG 读回也正确的
	// 位置。依次尝试各粗收敛起点，直到某个起点精化成功；全部失败则回退首个
	// 粗收敛结果（PNG 无损场景仍 100% 正确，JPEG 场景靠副本投票兜底）。
	var coarseFallback []int
	for _, start := range starts {
		coarse, ok := pocsRunAnchor(start, blkCb, blkCr, syncBits, dataBits, anchorCenterThresh)
		if !ok {
			continue
		}
		if coarseFallback == nil {
			coarseFallback = coarse
		}
		if polished, ok2 := pocsRunAnchorJPEG(coarse, plane, cbs, crs, w, h, ax, ay, syncBits, dataBits, 2); ok2 {
			writeBackBlock(plane, w, ax, ay, polished)
			return true
		}
	}
	if coarseFallback != nil {
		writeBackBlock(plane, w, ax, ay, coarseFallback)
		return true
	}
	// 全部起点未收敛：还原原始块，避免残留的局部同步/数据被提取端误判为锚点。
	writeBackBlock(plane, w, ax, ay, orig)
	return false
}

// pocsRunAnchor 从给定像素起点运行块级 POCS 迭代，收敛后返回最终像素块。
// 每轮分解 → 仅修正"桶错误或距中心超阈值"的系数（正确且居中的系数保持
// 不动，避免把已收敛系数重新扰动到错误桶）→ 逆变换 → clamp + RGB 往返
// 读回验证（roundTripY 用 JPEG 4:2:0 子采样色度模拟提取端真实链路）。
func pocsRunAnchor(blk []int, blkCb, blkCr []byte, syncBits, dataBits []byte, thresh int) ([]int, bool) {
	ll2 := make([]int, ll2Size*ll2Size)
	for iter := 0; iter < 40; iter++ {
		bands := decomposeHaar(blk, anchorBlock, anchorBlock, 2)
		// 同步区（LL2 低频，96 bit × syncVotes 票）：桶错误或近边界系数重新中心化。
		readLL2(blk, ll2)
		for i := 0; i < syncFieldBits; i++ {
			for v := 0; v < syncVotes; v++ {
				idx := i*syncVotes + v
				if anchExtract(ll2[idx]) != syncBits[i] || anchDistCenter(ll2[idx]) > thresh {
					ll2[idx] = anchQIM(ll2[idx], syncBits[i])
				}
			}
		}
		writeLL2(blk, ll2)
		// 数据区（LL2 剩余 + HL2 + LH2，votes 份冗余）：桶错误或近边界系数重新中心化。
		fixAnchorData(blk, bands, dataBits, thresh)
		recomposeHaar(blk, anchorBlock, anchorBlock, bands, 2)

		// 模拟提取端：clamp + RGB 往返后读回的 Y。
		readBack := make([]int, anchorBlock*anchorBlock)
		for i := range readBack {
			readBack[i] = int(roundTripY(clampByte(blk[i]), blkCb[i], blkCr[i]))
		}
		if verifyAnchorBlock(readBack, syncBits, dataBits) {
			// 写回嵌入后的像素块（blk），而非模拟读回（readBack）；
			// 读回仅是验证用的提取端模拟。
			return blk, true
		}
		blk = readBack
	}
	return nil, false
}

// pocsRunAnchorJPEG 从粗收敛像素块出发，用真实 JPEG 编码/解码模拟提取端
// 读取链路做精细收敛：每轮分解 → 测 JPEG 读回漂移 → 按"目标桶中心 - 漂移"
// 预补偿写入系数（使 JPEG 读回落在桶中心）→ 逆变换 → 再以块像素 + 精确
// 色度构建按 16 对齐的扩展区域做 jpeg.Encode(q75)+Decode，经 toNRGBA→
// toYCbCr 读回块内 Y → 验证位流。
//
// 与粗收敛不同，这里不做"把整块替换为 JPEG 读回"的投影：JPEG 读回含 DCT
// 量化噪声，直接替换会让系数域大范围失配、每轮需要海量重新修正且不收敛。
// 改为在系数域测量确定性漂移 d = 读回系数 - 写入系数，把写入值预置为
// target - d：若 d 稳定，读回系数 ≈ target（桶中心），既满足位流验证又
// 留足抗噪余量。验证通过则返回"嵌入后的像素块"（非 JPEG 读回）：JPEG
// 编码确定，同一像素块的再次压缩必然产出同一验证结果。
// 失败返回 nil, false，由调用方回退粗收敛结果（PNG 无损仍 100% 正确）。
func pocsRunAnchorJPEG(start []int, plane []int, cbs, crs []uint8, w, h, ax, ay int, syncBits, dataBits []byte, thresh int) ([]int, bool) {
	x0, y0 := ax-anchorBlock/2, ay-anchorBlock/2
	blk := make([]int, len(start))
	copy(blk, start)
	ll2 := make([]int, ll2Size*ll2Size)
	ll2r := make([]int, ll2Size*ll2Size)
	nSync := syncFieldBits * syncVotes
	nData := anchorDataBits * anchorDataVotes
	for iter := 0; iter < 60; iter++ {
		// 1) 分解当前块，快照写入系数；重组回像素供 JPEG。
		bands := decomposeHaar(blk, anchorBlock, anchorBlock, 2)
		readLL2(blk, ll2)
		wSync := append([]int(nil), ll2[:nSync]...)
		wData := make([]int, nData)
		readAnchorDataCoeff(blk, bands, wData)
		recomposeHaar(blk, anchorBlock, anchorBlock, bands, 2)

		// 2) JPEG 模拟读回并验证。
		readBack := jpegRoundTripAnchorBlock(blk, plane, cbs, crs, w, h, x0, y0)
		if readBack == nil {
			return nil, false
		}
		if verifyAnchorBlock(readBack, syncBits, dataBits) {
			return blk, true
		}

		// 3) 读回系数，测漂移。
		bandsR := decomposeHaar(readBack, anchorBlock, anchorBlock, 2)
		readLL2(readBack, ll2r)
		rSync := append([]int(nil), ll2r[:nSync]...)
		rData := make([]int, nData)
		readAnchorDataCoeff(readBack, bandsR, rData)

		// 4) 补偿：new = target - drift（drift = r - w）。系数写值允许落在
		//    任意桶，只要 JPEG 读回落在目标桶中心即可。
		bandsN := decomposeHaar(blk, anchorBlock, anchorBlock, 2)
		readLL2(blk, ll2)
		refs := anchorDataRefs(blk, bandsN)
		for i := 0; i < syncFieldBits; i++ {
			for v := 0; v < syncVotes; v++ {
				k := i*syncVotes + v
				ll2[k] = anchQIM(ll2[k], syncBits[i]) - (rSync[k] - wSync[k])
			}
		}
		writeLL2(blk, ll2)
		for k := 0; k < nData; k++ {
			*refs[k] = anchQIM(*refs[k], dataBits[k%anchorDataBits]) - (rData[k] - wData[k])
		}
		recomposeHaar(blk, anchorBlock, anchorBlock, bandsN, 2)
	}
	return nil, false
}

// jpegRoundTripAnchorBlock 模拟真实 JPEG 压缩链路的块级 Y 读回。
//
// 提取端对压缩后载体的读取链路是：jpeg.Decode → 转 NRGBA → toYCbCr → Y。
// 模拟完全复刻该链路：以块像素（当前 POCS 状态）+ 精确色度构建 NRGBA
// （与嵌入写回 PNG 的像素一致），jpeg.Encode(anchorJPEGSimQuality) 编码、
// Decode 解码、转 NRGBA、toYCbCr 后取块内 Y。
//
// 区域选择：锚点块左上角 (x0,y0) 未必落在 8×8/16×16 网格上，直接对 64×64
// 独立块编码会让边缘 DCT/MCU 的像素来源与整图压缩不一致。因此把区域按 16
// 对齐扩展到覆盖锚点块的最小矩形（rw/rh 为 16 的倍数），区域内每个 DCT 块
// 与色度 MCU 的像素来源和整图编码完全一致（JPEG 量化只依赖块内像素），
// 块级模拟与整图压缩后的解码结果一致。块外像素取 plane（当前嵌入状态，
// 已含先前锚点的写回）、块内取 blk（本锚点当前 POCS 状态）；Cb/Cr 一律取
// 精确色度（JPEG 编码端自身会做 4:2:0 子采样）。区域贴图像边缘时边缘复制
// 与整图编码一致，无需特殊处理。
//
// 返回读回的 64×64 Y 平面；编码/解码失败或区域尺寸异常返回 nil。
func jpegRoundTripAnchorBlock(blk []int, plane []int, cbs, crs []uint8, w, h, x0, y0 int) []int {
	rx := x0 &^ 15
	ry := y0 &^ 15
	rw := ((x0 + anchorBlock + 15) &^ 15) - rx
	rh := ((y0 + anchorBlock + 15) &^ 15) - ry
	if rx < 0 {
		rw += rx
		rx = 0
	}
	if ry < 0 {
		rh += ry
		ry = 0
	}
	if rx+rw > w {
		rw = w - rx
	}
	if ry+rh > h {
		rh = h - ry
	}
	if rw < anchorBlock || rh < anchorBlock || rw&1 != 0 || rh&1 != 0 {
		return nil
	}

	img := image.NewNRGBA(image.Rect(0, 0, rw, rh))
	ex0, ey0 := x0-rx, y0-ry
	for y := 0; y < rh; y++ {
		for x := 0; x < rw; x++ {
			var yv int
			if ex0 <= x && x < ex0+anchorBlock && ey0 <= y && y < ey0+anchorBlock {
				yv = blk[(y-ey0)*anchorBlock+(x-ex0)]
			} else {
				yv = plane[(ry+y)*w+rx+x]
			}
			i := (ry+y)*w + rx + x
			r, g, b := yuvToRGB(clampByte(yv), cbs[i], crs[i])
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: anchorJPEGSimQuality}); err != nil {
		return nil
	}
	dec, err := jpeg.Decode(&buf)
	if err != nil {
		return nil
	}
	b := dec.Bounds()
	nrgba := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			nrgba.SetNRGBA(x, y, color.NRGBAModel.Convert(dec.At(x+b.Min.X, y+b.Min.Y)).(color.NRGBA))
		}
	}
	yuv := toYCbCr(nrgba)
	out := make([]int, anchorBlock*anchorBlock)
	for y := 0; y < anchorBlock; y++ {
		for x := 0; x < anchorBlock; x++ {
			out[y*anchorBlock+x] = int(yuv[(ey0+y)*rw+ex0+x].Y)
		}
	}
	return out
}

// verifyAnchorBlock 验证读回像素的同步区（LL1 投票）与数据区（votes 份多数投票）是否与目标一致。
// 注意：decomposeHaar 是原地分解，必须先拷贝，否则会破坏调用方的像素平面
// （POCS 循环以读回平面为下一轮起点，绝不能在这里被改成系数域）。
func verifyAnchorBlock(pix []int, syncBits, dataBits []byte) bool {
	work := make([]int, len(pix))
	copy(work, pix)
	bands := decomposeHaar(work, anchorBlock, anchorBlock, 2)
	ll2 := make([]int, ll2Size*ll2Size)
	readLL2(work, ll2)
	for i := 0; i < syncFieldBits; i++ {
		ones := 0
		for v := 0; v < syncVotes; v++ {
			ones += int(anchExtract(ll2[i*syncVotes+v]))
		}
		if (ones*2 > syncVotes) != (syncBits[i] == 1) {
			return false
		}
	}
	coeff := make([]int, anchorDataBits*anchorDataVotes)
	readAnchorDataCoeff(work, bands, coeff)
	for i := 0; i < anchorDataBits; i++ {
		ones := 0
		for v := 0; v < anchorDataVotes; v++ {
			ones += int(anchExtract(coeff[v*anchorDataBits+i]))
		}
		if (ones*2 > anchorDataVotes) != (dataBits[i] == 1) {
			return false
		}
	}
	return true
}

// readLL2 从分解后的块平面读取 LL2 子带（左上 16×16 区域）。
func readLL2(blk []int, out []int) {
	for y := 0; y < ll2Size; y++ {
		copy(out[y*ll2Size:(y+1)*ll2Size], blk[y*anchorBlock:y*anchorBlock+ll2Size])
	}
}

// writeLL2 将 LL2 子带写回块平面。
func writeLL2(blk []int, ll2 []int) {
	for y := 0; y < ll2Size; y++ {
		copy(blk[y*anchorBlock:y*anchorBlock+ll2Size], ll2[y*ll2Size:(y+1)*ll2Size])
	}
}

// anchorBlockPixels 提取锚点邻域块。
func anchorBlockPixels(plane []int, w, ax, ay int) []int {
	x0, y0 := ax-anchorBlock/2, ay-anchorBlock/2
	blk := make([]int, anchorBlock*anchorBlock)
	for y := 0; y < anchorBlock; y++ {
		copy(blk[y*anchorBlock:(y+1)*anchorBlock], plane[(y0+y)*w+x0:(y0+y)*w+x0+anchorBlock])
	}
	return blk
}

// writeBackBlock 写回锚点邻域块。
func writeBackBlock(plane []int, w, ax, ay int, blk []int) {
	x0, y0 := ax-anchorBlock/2, ay-anchorBlock/2
	for y := 0; y < anchorBlock; y++ {
		copy(plane[(y0+y)*w+x0:(y0+y)*w+x0+anchorBlock], blk[y*anchorBlock:(y+1)*anchorBlock])
	}
}

// anchorBlockBytes 提取锚点邻域块的单通道字节平面。
func anchorBlockBytes(src []uint8, w, ax, ay int) []byte {
	x0, y0 := ax-anchorBlock/2, ay-anchorBlock/2
	out := make([]byte, anchorBlock*anchorBlock)
	for y := 0; y < anchorBlock; y++ {
		copy(out[y*anchorBlock:(y+1)*anchorBlock], src[(y0+y)*w+x0:(y0+y)*w+x0+anchorBlock])
	}
	return out
}

// writeVoted 每个 bit 以 votes 次冗余写入系数序列。
func writeVoted(coeff []int, bits []byte, votes int) {
	for i := range bits {
		for v := 0; v < votes; v++ {
			coeff[i*votes+v] = anchQIM(coeff[i*votes+v], bits[i])
		}
	}
}

// buildSyncBits 生成同步区 96 bit：同步码(64) + 片数(8) + ID(8) + CRC16(16)。
// 同步码由密码种子确定性生成，嵌入/提取两侧一致。
func buildSyncBits(seed []byte, id, chunks int) []byte {
	out := make([]byte, syncFieldBits)
	rng := NewRNG(seed)
	for i := 0; i < syncPatternBits; i++ {
		out[i] = rng.NextBit()
	}
	for i := 0; i < 8; i++ {
		out[syncPatternBits+i] = byte((chunks >> uint(7-i)) & 1)
		out[syncPatternBits+8+i] = byte((id >> uint(7-i)) & 1)
	}
	crc := crc16Sync(id, chunks)
	for i := 0; i < 16; i++ {
		out[syncPatternBits+16+i] = byte((crc >> uint(15-i)) & 1)
	}
	return out
}

// ---- 提取辅助 ----

// syncHit 一个已确认的锚点。
type syncHit struct {
	x, y   int
	id     int
	chunks int
	dist   int // 同步模式距离（对齐质量，越小越好）
}

// findSyncHits 在灰度图上检测 FAST 角点并扫描同步区，返回确认锚点。
//
// 扫描策略（解决 QIM 嵌入 + JPEG 压缩后锚点角点响应下降、被
// "响应降序截断"丢弃的问题）：
//  1. 全量扫描：纹理丰富的图上 FAST 可检出数万点，锚点嵌入后角点响应
//     可能排在数千名开外，不能按响应降序截断，需全量扫描（上限
//     syncScanLimit，该值足够大以覆盖任意锚点）；也不做局部去重——
//     密集纹理下锚点角点旁 8px 内常有响应更高的相邻角点，去重会误删
//     锚点自身的点。
//  2. 偏移细化：JPEG 压缩会使角点检测位置漂移 ±1~2px，LL2 低频子带对
//     窗口错位敏感，1px 偏移即可能翻转数个同步位。对同步模式距离接近
//     的候选，探测 ±2px 邻域取最佳对齐，显著提高锚点恢复率。
func findSyncHits(gray []byte, w, h int, seed []byte) []syncHit {
	pts := vision.FAST(gray, w, h, 0)
	if len(pts) > syncScanLimit {
		pts = pts[:syncScanLimit]
	}

	ref := make([]byte, syncPatternBits)
	rng := NewRNG(seed)
	for i := 0; i < syncPatternBits; i++ {
		ref[i] = rng.NextBit()
	}

	hits := make([]syncHit, 0, 8)
	for _, p := range pts {
		if p.X < anchorEdge || p.Y < anchorEdge || p.X > w-anchorEdge-1 || p.Y > h-anchorEdge-1 {
			continue
		}
		bits88, ok := readSyncBits(gray, w, h, p.X, p.Y)
		if !ok {
			continue
		}
		bestBits, bestD := bits88, hammingBits(bits88[:syncPatternBits], ref)
		bestX, bestY := p.X, p.Y
		// 阶段 A：中心 + 4 个 ±1 邻域（5 次读取）。
		// JPEG 使角点检测相对嵌入锚点漂移 1~2px：漂移 1px 时粗读模式距离
		// 可能高达 ~30，必须先把 ±1 探测纳入门槛判定，否则锚点被提前拒绝。
		// 随机（非锚点）块 5 次读取距离均≈32，此处即被批量拒绝。
		// 同步与数据共用同一 64×64 块：细化得到的最佳位置同时是数据区
		// 的最佳读取位置，必须写回 hit（此前仅返回 FAST 点导致数据错位）。
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				b2, ok2 := readSyncBits(gray, w, h, p.X+dx, p.Y+dy)
				if !ok2 {
					continue
				}
				d2 := hammingBits(b2[:syncPatternBits], ref)
				if d2 < bestD {
					bestBits, bestD, bestX, bestY = b2, d2, p.X+dx, p.Y+dy
				}
			}
		}
		if bestD > syncTol+12 {
			continue
		}
		// 阶段 B：补充 ±2 其余位置（对齐 2px 漂移的罕见情况）。
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				b2, ok2 := readSyncBits(gray, w, h, p.X+dx, p.Y+dy)
				if !ok2 {
					continue
				}
				d2 := hammingBits(b2[:syncPatternBits], ref)
				if d2 < bestD {
					bestBits, bestD, bestX, bestY = b2, d2, p.X+dx, p.Y+dy
				}
			}
		}
		if bestD > syncTol {
			continue
		}
		id, chunks, crcOK := parseSyncCorrect(bestBits)
		if !crcOK {
			continue
		}
		hits = append(hits, syncHit{x: bestX, y: bestY, id: id, chunks: chunks, dist: bestD})
	}
	// 去重：同一锚点（同 id 且位置在 4px 内）只保留同步对齐最佳者。
	// FAST 会在锚点邻域检出多个角点，细化后都能通过同步校验；若全部保留，
	// 轻微错位的副本会以高错误率的数据污染多数表决（旋转/裁剪等无损攻击下
	// 尤其明显：锚点块经精确旋转后角点检测可能落在 1px 偏移处）。
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].dist != hits[j].dist {
			return hits[i].dist < hits[j].dist
		}
		return hits[i].id < hits[j].id
	})
	dedup := hits[:0]
	for _, h := range hits {
		dup := false
		for _, k := range dedup {
			if k.id != h.id {
				continue
			}
			dx, dy := k.x-h.x, k.y-h.y
			if dx*dx+dy*dy <= 16 {
				dup = true
				break
			}
		}
		if !dup {
			dedup = append(dedup, h)
		}
	}
	return dedup
}

// readSyncBits 从锚点块读取 88 bit 同步区（LL2 低频子带，投票读出）。
// computeLL2 直接从灰度平面计算锚点块（左上角 x0,y0，尺寸 64×64）的
// 二级 LL 子带（16×16），与 decomposeHaar(levels=2)+readLL2 输出完全一致，
// 但省去块平面拷贝与完整分解。同步区只依赖 LL2，故同步扫描数千候选点时
// 用本函数可避免每次分配 32KB 块平面与完整 DWT 的开销。
func computeLL2(gray []byte, w, h, x0, y0 int) (ll2 [ll2Size * ll2Size]int) {
	// 一级水平低通（每行相邻对取均值）。
	var srow [anchorBlock][anchorBlock / 2]int
	for y := 0; y < anchorBlock; y++ {
		row := (y0 + y) * w
		for x := 0; x < anchorBlock/2; x++ {
			col := x0 + 2*x
			srow[y][x] = (int(gray[row+col]) + int(gray[row+col+1])) >> 1
		}
	}
	// 一级垂直低通（LL1: 32×32）。
	var ll1 [anchorBlock / 2][anchorBlock / 2]int
	for y := 0; y < anchorBlock/2; y++ {
		for x := 0; x < anchorBlock/2; x++ {
			ll1[y][x] = (srow[2*y][x] + srow[2*y+1][x]) >> 1
		}
	}
	// 二级水平低通（写回 ll1 前 16 列）。
	for y := 0; y < anchorBlock/2; y++ {
		for x := 0; x < ll2Size; x++ {
			ll1[y][x] = (ll1[y][2*x] + ll1[y][2*x+1]) >> 1
		}
	}
	// 二级垂直低通（LL2: 16×16）。
	for y := 0; y < ll2Size; y++ {
		for x := 0; x < ll2Size; x++ {
			ll2[y*ll2Size+x] = (ll1[2*y][x] + ll1[2*y+1][x]) >> 1
		}
	}
	return ll2
}

func readSyncBits(gray []byte, w, h, ax, ay int) ([]byte, bool) {
	x0, y0 := ax-anchorBlock/2, ay-anchorBlock/2
	if x0 < 0 || y0 < 0 || x0+anchorBlock > w || y0+anchorBlock > h {
		return nil, false
	}
	ll2 := computeLL2(gray, w, h, x0, y0)
	out := make([]byte, syncFieldBits)
	for i := 0; i < syncFieldBits; i++ {
		ones := 0
		for v := 0; v < syncVotes; v++ {
			ones += int(anchExtract(ll2[i*syncVotes+v]))
		}
		if ones*2 > syncVotes {
			out[i] = 1
		}
	}
	return out, true
}

// parseSync 解析同步区：片数、ID、CRC16 校验。
func parseSync(bits []byte) (id, chunks int, ok bool) {
	for i := 0; i < 8; i++ {
		chunks = chunks<<1 | int(bits[syncPatternBits+i])
		id = id<<1 | int(bits[syncPatternBits+8+i])
	}
	var crc uint16
	for i := 0; i < 16; i++ {
		crc = crc<<1 | uint16(bits[syncPatternBits+16+i])
	}
	return id, chunks, crc == crc16Sync(id, chunks)
}

// parseSyncCorrect 解析同步区；CRC 失败时对 16 位头部（chunks+id）做
// 有界 1~3 bit 翻转搜索，纠正 JPEG 压缩在 LL2 低频同步位引入的少量错误。
// 安全性：仅在模式距离 ≤ syncTol 的候选上调用，非锚点块的模式匹配概率
// 约为 2^-58，误纠正（随机头恰好通过 CRC16）概率 ≈ 696/2^16，两者乘积
// 可忽略，不会引入伪锚点；且纠正结果 id 必须 <256、chunks 落在 [1,16]。
func parseSyncCorrect(bits []byte) (id, chunks int, ok bool) {
	if id, chunks, ok = parseSync(bits); ok {
		return id, chunks, true
	}
	head := uint16(0)
	for i := 0; i < 16; i++ {
		head = head<<1 | uint16(bits[syncPatternBits+i])
	}
	var crc uint16
	for i := 0; i < 16; i++ {
		crc = crc<<1 | uint16(bits[syncPatternBits+16+i])
	}
	valid := func(h uint16) bool {
		if crc16Sync(int(h&0xFF), int(h>>8)) != crc {
			return false
		}
		id = int(h & 0xFF)
		chunks = int(h >> 8)
		return id < 256 && chunks >= 1 && chunks <= 16
	}
	for i := 0; i < 16; i++ { // 单 bit
		if valid(head ^ (1 << uint(i))) {
			return id, chunks, true
		}
	}
	for i := 0; i < 16; i++ { // 双 bit
		for j := i + 1; j < 16; j++ {
			if valid(head ^ (1 << uint(i)) ^ (1 << uint(j))) {
				return id, chunks, true
			}
		}
	}
	for i := 0; i < 16; i++ { // 三 bit
		for j := i + 1; j < 16; j++ {
			for k := j + 1; k < 16; k++ {
				if valid(head ^ (1 << uint(i)) ^ (1 << uint(j)) ^ (1 << uint(k))) {
					return id, chunks, true
				}
			}
		}
	}
	return 0, 0, false
}

// readAnchorData 读取锚点块数据区字节（votes 份冗余多数投票）。
func readAnchorData(gray []byte, w, h, ax, ay int) []byte {
	x0, y0 := ax-anchorBlock/2, ay-anchorBlock/2
	blk := make([]int, anchorBlock*anchorBlock)
	for y := 0; y < anchorBlock; y++ {
		for x := 0; x < anchorBlock; x++ {
			blk[y*anchorBlock+x] = int(gray[(y0+y)*w+x0+x])
		}
	}
	bands := decomposeHaar(blk, anchorBlock, anchorBlock, 2)
	coeff := make([]int, anchorDataBits*anchorDataVotes)
	readAnchorDataCoeff(blk, bands, coeff)
	out := make([]byte, anchorDataBytes)
	for i := 0; i < anchorDataBits; i++ {
		ones := 0
		for v := 0; v < anchorDataVotes; v++ {
			ones += int(anchExtract(coeff[v*anchorDataBits+i]))
		}
		if ones*2 > anchorDataVotes {
			out[i/8] |= 1 << uint(7-i%8)
		}
	}
	return out
}

// voteChunk 对片内副本逐位多数投票（副本不足时取第一个；无副本返回全 0）。
func voteChunk(copies [][]byte) []byte {
	out := make([]byte, anchorDataBytes)
	if len(copies) == 0 {
		return out
	}
	if len(copies) == 1 {
		return copies[0]
	}
	for b := 0; b < anchorDataBytes; b++ {
		var v byte
		for j := 0; j < 8; j++ {
			ones := 0
			for _, c := range copies {
				ones += int((c[b] >> uint(7-j)) & 1)
			}
			if ones*2 > len(copies) {
				v |= 1 << uint(7-j)
			}
		}
		out[b] = v
	}
	return out
}

// modeChunks 取各锚点同步区携带片数的众数。
func modeChunks(hits []syncHit) int {
	if len(hits) == 0 {
		return 0
	}
	cnt := map[int]int{}
	for _, hit := range hits {
		cnt[hit.chunks]++
	}
	best, bestN := 0, -1
	for c, n := range cnt {
		if n > bestN {
			best, bestN = c, n
		}
	}
	return best
}

// ---- 锚点选择 ----

// kpAnchor 已选锚点。
type kpAnchor struct{ x, y int }

// selectAnchors 响应降序 + 最小间距 + 边界约束，取前 anchorMax 个锚点。
func (a *anchored) selectAnchors(plane []int, w, h int) []kpAnchor {
	gray := a.grayPlaneOf(plane, w, h)
	pts := vision.FAST(gray, w, h, 0)
	return pickAnchors(pts, w, h)
}

// countAnchorCandidates 统计可锚定候选数（容量估算用）。
func countAnchorCandidates(gray []byte, w, h int) int {
	return len(pickAnchors(vision.FAST(gray, w, h, 0), w, h))
}

// pickAnchors 过滤边界并施加最小间距约束。
func pickAnchors(pts []vision.KeyPoint, w, h int) []kpAnchor {
	anchors := make([]kpAnchor, 0, anchorMax)
	for _, p := range pts {
		if p.X < anchorEdge || p.Y < anchorEdge || p.X > w-anchorEdge-1 || p.Y > h-anchorEdge-1 {
			continue
		}
		ok := true
		for _, ap := range anchors {
			dx, dy := p.X-ap.x, p.Y-ap.y
			if dx*dx+dy*dy < anchorMinDist*anchorMinDist {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		anchors = append(anchors, kpAnchor{x: p.X, y: p.Y})
		if len(anchors) >= anchorMax {
			break
		}
	}
	return anchors
}

// ---- 通用 ----

// chroma420Avg 计算 (x,y) 为左上角的 2×2 块的 Cb/Cr 均值（JPEG 4:2:0 子采样）。
// x、y 须为偶数（调用方保证）。越界像素按复制边缘处理。
// buildSubsampledChroma 构建 JPEG 4:2:0 色度子采样模型：对每个像素，取其
// 所在 2×2 块的色度均值（解码端上采样后的读回色度）。
func buildSubsampledChroma(cbs, crs []uint8, w, h int) (subCbs, subCrs []uint8) {
	subCbs = make([]uint8, w*h)
	subCrs = make([]uint8, w*h)
	for y := 0; y < h; y++ {
		by := y &^ 1
		for x := 0; x < w; x++ {
			bx := x &^ 1
			subCbs[y*w+x], subCrs[y*w+x] = chroma420Avg(cbs, crs, w, h, bx, by)
		}
	}
	return
}

func chroma420Avg(cbs, crs []uint8, w, h, x, y int) (uint8, uint8) {
	var cbSum, crSum int
	n := 0
	for dy := 0; dy < 2; dy++ {
		yy := y + dy
		if yy >= h {
			yy = h - 1
		}
		for dx := 0; dx < 2; dx++ {
			xx := x + dx
			if xx >= w {
				xx = w - 1
			}
			i := yy*w + xx
			cbSum += int(cbs[i])
			crSum += int(crs[i])
			n++
		}
	}
	return uint8((cbSum + n/2) / n), uint8((crSum + n/2) / n)
}

// grayPlaneOf 由压缩 int 平面生成灰度字节平面。
func (a *anchored) grayPlaneOf(plane []int, w, h int) []byte {
	gray := make([]byte, w*h)
	for i := range gray {
		gray[i] = clampByte(plane[i])
	}
	return gray
}

// anchQIM QIM 嵌入（间隔 anchorQuant，桶中心化）。
func anchQIM(k int, bit byte) int {
	sign := 1
	abs := k
	if k < 0 {
		sign, abs = -1, -k
	}
	target := abs / anchorQuant
	if target&1 != int(bit&1) {
		if target > 0 {
			target--
		} else {
			target++
		}
	}
	return sign * (target*anchorQuant + anchorQuant/2)
}

// anchExtract 读取桶奇偶位。
func anchExtract(k int) byte {
	abs := k
	if k < 0 {
		abs = -k
	}
	return byte((abs / anchorQuant) & 1)
}

// hammingBits 计算两个 bit 数组的汉明距离。
func hammingBits(a, b []byte) int {
	d := 0
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		d += int(a[i] ^ b[i])
	}
	return d
}

// crc16Sync CRC-16/CCITT（多项式 0x1021），用于同步区 ID/片数完整性校验。
func crc16Sync(vals ...int) uint16 {
	crc := uint16(0xFFFF)
	for _, v := range vals {
		crc ^= uint16(v) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
