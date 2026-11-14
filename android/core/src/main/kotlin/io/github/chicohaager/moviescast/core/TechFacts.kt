package io.github.chicohaager.moviescast.core

/**
 * What a film's file holds, for the line of facts on its page as Infuse shows it ("4K · HDR · TrueHD 7.1") and the
 * file line at its foot. Movies names little of it: for an MKV only the codecs (no size, no channels, no HDR; measured
 * 2026-11-14), so the app reads the rest from the file's own header (the player's extractors, without playing) and
 * puts the two together with [mergedWith]: the server's value where it has one.
 */
data class TechFacts(
    val width: Int = 0,
    val height: Int = 0,
    val hdr: Hdr = Hdr.UNKNOWN,
    /** The first audio track's codec as the servers write it (`truehd`, `eac3`, `dts` …), "" unknown. */
    val audioCodec: String = "",
    val channels: Int = 0,
    val frameRate: Float = 0f,
    /** Bits per second of the whole file, 0 unknown. */
    val bitrate: Long = 0,
) {
    enum class Hdr { UNKNOWN, SDR, HDR10, HLG, DOLBY_VISION }

    /** Each value from this one where it is known, else from [other]. */
    fun mergedWith(other: TechFacts): TechFacts = TechFacts(
        width = if (width > 0) width else other.width,
        height = if (height > 0) height else other.height,
        hdr = if (hdr != Hdr.UNKNOWN) hdr else other.hdr,
        audioCodec = audioCodec.ifEmpty { other.audioCodec },
        channels = if (channels > 0) channels else other.channels,
        frameRate = if (frameRate > 0f) frameRate else other.frameRate,
        bitrate = if (bitrate > 0) bitrate else other.bitrate,
    )

    /** Whether the file's header has to be read: something the line shows is missing. */
    val incomplete: Boolean get() = width <= 0 || hdr == Hdr.UNKNOWN || audioCodec.isEmpty() || channels <= 0

    /** "4K", "1080p", "720p", "SD" by the picture's width (a scope film of 2046×852 is 1080p); null unknown. */
    val resolution: String?
        get() = when {
            width <= 0 -> null
            width >= 3200 -> "4K"
            width >= 1800 -> "1080p"
            width >= 1200 -> "720p"
            else -> "SD"
        }

    /** "HDR", "HLG", "Dolby Vision"; null for SDR and unknown. */
    val hdrLabel: String?
        get() = when (hdr) {
            Hdr.HDR10 -> "HDR"
            Hdr.HLG -> "HLG"
            Hdr.DOLBY_VISION -> "Dolby Vision"
            else -> null
        }

    /** "TrueHD 7.1", "Dolby Digital+ 5.1", "AAC 2.0"; the codec alone without channels; null unknown. */
    val audioLabel: String?
        get() {
            if (audioCodec.isEmpty()) return null
            val name = when (audioCodec.lowercase()) {
                "truehd", "mlp" -> "TrueHD"
                "eac3", "ec-3" -> "Dolby Digital+"
                "ac3", "ac-3" -> "Dolby Digital"
                "dts" -> "DTS"
                "dtshd", "dts-hd" -> "DTS-HD"
                "aac" -> "AAC"
                "flac" -> "FLAC"
                "opus" -> "Opus"
                "mp3" -> "MP3"
                "pcm", "lpcm" -> "PCM"
                else -> audioCodec.uppercase()
            }
            val layout = when (channels) {
                0 -> null
                1 -> "1.0"
                2 -> "2.0"
                3 -> "2.1"
                6 -> "5.1"
                7 -> "6.1"
                8 -> "7.1"
                else -> "$channels ch"
            }
            return if (layout == null) name else "$name $layout"
        }

    /** "23.976 fps", "25 fps"; null unknown. */
    val frameRateLabel: String?
        get() {
            if (frameRate <= 0f) return null
            val rounded = Math.round(frameRate * 1000) / 1000.0
            val text = if (rounded == Math.floor(rounded)) rounded.toLong().toString() else "%.3f".format(java.util.Locale.ROOT, rounded).trimEnd('0')
            return "$text fps"
        }

    /** "75 Mbit/s", "12.7 Mbit/s", "800 kbit/s"; null unknown. */
    val bitrateLabel: String?
        get() = when {
            bitrate <= 0 -> null
            bitrate >= 20_000_000 -> "${Math.round(bitrate / 1e6)} Mbit/s"
            bitrate >= 1_000_000 -> "%.1f Mbit/s".format(java.util.Locale.ROOT, bitrate / 1e6)
            else -> "${Math.round(bitrate / 1e3)} kbit/s"
        }

    companion object {
        /**
         * What the server tells of an item; the bitrate, if the server names none, from the file's size and running
         * time (Movies names none for an MKV).
         */
        fun of(item: MediaItem): TechFacts = TechFacts(
            width = item.width,
            height = item.height,
            // Jellyfin's VideoRangeType: SDR, HDR10, HDR10Plus, HLG, DOVI, DOVIWithHDR10 …, Unknown
            hdr = item.videoRange.uppercase().let { range ->
                when {
                    range.isEmpty() || range == "UNKNOWN" -> TechFacts.Hdr.UNKNOWN
                    range == "SDR" -> TechFacts.Hdr.SDR
                    range.startsWith("DOVI") -> TechFacts.Hdr.DOLBY_VISION
                    range == "HLG" -> TechFacts.Hdr.HLG
                    else -> TechFacts.Hdr.HDR10
                }
            },
            audioCodec = item.audio.firstOrNull()?.codec.orEmpty(),
            channels = item.audio.firstOrNull()?.channels ?: 0,
            frameRate = item.frameRate,
            bitrate = if (item.bitrate > 0) item.bitrate
            else if (item.sizeBytes > 0 && item.runtimeSeconds > 0) item.sizeBytes * 8 / item.runtimeSeconds
            else 0,
        )
    }
}
