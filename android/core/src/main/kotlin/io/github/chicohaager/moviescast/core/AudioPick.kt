package io.github.chicohaager.moviescast.core

/**
 * Which sound track plays when the viewer has not chosen one, ranked as Kodi ranks them
 * (`xbmc/utils/StreamUtils.cpp`, `CompareAudioPreference`, `CompareAudioQuality`, `GetCodecPriority`):
 * 1. the language of the device, as the app asked of Media3 before (Kodi: the language the person set),
 * 2. the track the file marks as default (Kodi: `videoplayer.preferdefaultflag`, on by default),
 * 3. quality: a surround track before a stereo or mono one; among surround tracks the better codec, then more channels;
 *    up to stereo more channels, then the better codec,
 * 4. the file's order.
 * Media3 put a renderer's preference before the file's default: the German AC3 mono track of a film played instead of
 * its German DTS-HD 5.1 track marked default (Google TV Streamer, 2026-11-16).
 */
object AudioPick {
    data class Track(val language: String?, val mimeType: String?, val channels: Int, val isDefault: Boolean)

    /** The index in [tracks] of the one to play, or -1 when there is none. */
    fun best(tracks: List<Track>, deviceLanguage: String): Int {
        if (tracks.isEmpty()) return -1
        val order = compareByDescending<IndexedValue<Track>> { languageMatches(it.value.language, deviceLanguage) }
            .thenByDescending { it.value.isDefault }
            .thenComparator { a, b -> -compareQuality(a.value, b.value) }
            .thenBy { it.index }
        return tracks.withIndex().sortedWith(order).first().index
    }

    private fun languageMatches(track: String?, device: String): Boolean {
        val t = track?.substringBefore('-')?.lowercase() ?: return false
        val d = device.substringBefore('-').lowercase()
        return t.isNotEmpty() && (t == d || THREE_LETTERS[t] == d || THREE_LETTERS[d] == t)
    }

    /** > 0 when [a] is the better listen, as Kodi's CompareAudioQuality. */
    fun compareQuality(a: Track, b: Track): Int {
        val ca = maxOf(0, a.channels)
        val cb = maxOf(0, b.channels)
        val pa = codecPriority(a.mimeType)
        val pb = codecPriority(b.mimeType)
        val surroundA = ca > 2
        val surroundB = cb > 2
        if (surroundA != surroundB) return if (surroundA) 1 else -1
        return if (surroundA) {
            if (pa != pb) pa.compareTo(pb) else ca.compareTo(cb)
        } else {
            if (ca != cb) ca.compareTo(cb) else pa.compareTo(pb)
        }
    }

    /** Kodi's GetCodecPriority for the names Media3 gives; an unknown format ranks 0. */
    fun codecPriority(mimeType: String?): Int = when (mimeType?.substringBefore(';')) {
        "audio/true-hd" -> 12
        "audio/flac", "audio/alac" -> 11
        "audio/vnd.dts.uhd" -> 13
        "audio/vnd.dts.hd" -> 9
        "audio/eac3-joc" -> 8
        "audio/raw" -> 7
        "audio/ac4" -> 6
        "audio/eac3", "audio/opus" -> 5
        "audio/vnd.dts" -> 4
        "audio/mp4a-latm", "audio/vorbis" -> 3
        "audio/ac3" -> 2
        "audio/mpeg", "audio/mpeg-L2" -> 1
        else -> 0
    }

    // Matroska and Movies name languages in three letters (ger, deu, eng), Android in two
    private val THREE_LETTERS = mapOf(
        "ger" to "de", "deu" to "de", "eng" to "en", "fre" to "fr", "fra" to "fr", "spa" to "es", "ita" to "it",
        "dut" to "nl", "nld" to "nl", "pol" to "pl", "por" to "pt", "rus" to "ru", "swe" to "sv", "tur" to "tr",
        "jpn" to "ja", "kor" to "ko", "chi" to "zh", "zho" to "zh", "cze" to "cs", "ces" to "cs", "hun" to "hu",
        "dan" to "da", "fin" to "fi", "nor" to "no",
    )
}
