package peer

func PickPiece(bitfield []byte, totalPieces uint32) (uint32, bool) {
	for i := uint32(0); i < totalPieces; i++ {
		byteIndex := i / 8
		bitIndex := 7 - (i % 8)

		if byteIndex >= uint32(len(bitfield)) {
			return 0, false
		}

		if (bitfield[byteIndex]>>bitIndex)&1 == 1 {
			return i, true
		}
	}
	return 0, false
}
