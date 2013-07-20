package cadastre

const CadastreEncodedFavicon = "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"
