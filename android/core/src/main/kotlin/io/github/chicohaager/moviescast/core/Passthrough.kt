package io.github.chicohaager.moviescast.core

/**
 * Which encoded sound the app may hand on unchanged over HDMI (passthrough), per format, as the person set it.
 *
 * Why (measured 2026-11-13 on a Fire TV Stick 4K): the stick says it can pass DTS-HD on, Media3 then sends it out
 * unchanged, and the TV behind it plays nothing; its own audio policy names AC3 only for HDMI. Jellyfin's Android TV
 * app has a setting per format (AC3, E-AC3, DTS, TrueHD) for that, and Kodi a switch per format. Here a format is
 * either left to the device or turned off: the app then decodes it to PCM itself (a decoder of
 * the device, else FFmpeg), which every TV plays; turned off is the default ([decode]). Forcing passthrough the device does not report is not offered:
 * Media3's audio sink would refuse it.
 */
object Passthrough {
    enum class Format(val key: String, internal val mimeTypes: Set<String>) {
        AC3("ac3", setOf("audio/ac3")),
        EAC3("eac3", setOf("audio/eac3", "audio/eac3-joc")),
        // Media3 1.11.1 MimeTypes: AUDIO_DTS, AUDIO_DTS_HD, AUDIO_DTS_EXPRESS, AUDIO_DTS_UHD_P2
        DTS("dts", setOf("audio/vnd.dts", "audio/vnd.dts.hd", "audio/vnd.dts.hd;profile=lbr", "audio/vnd.dts.uhd;profile=p2")),
        TRUEHD("truehd", setOf("audio/true-hd")),
    }

    /** The setting that decides about [mimeType], or null for sound that is no such format (PCM, AAC, …). */
    fun formatOf(mimeType: String?): Format? = Format.entries.firstOrNull { mimeType != null && mimeType in it.mimeTypes }

    /** False when [mimeType] is a format the person turned off: then it must be decoded, never handed on. */
    fun mayPassOn(mimeType: String?, off: Set<Format>): Boolean = formatOf(mimeType)?.let { it !in off } ?: true

    /**
     * The formats turned off, as kept in the settings ("dts,truehd"; "" = every format passed on); unknown names are
     * dropped. Nothing kept ([stored] null) means every format is decoded in the app, as Kodi 21 ships
     * (system/settings/settings.xml: audiooutput.passthrough and the per-format switches default false): the sound
     * then plays on any TV, and whoever has a receiver that takes a format turns its passing on.
     */
    fun decode(stored: String?): Set<Format> =
        if (stored == null) Format.entries.toSet()
        else stored.split(',').mapNotNull { name -> Format.entries.firstOrNull { it.key == name.trim() } }.toSet()

    fun encode(off: Set<Format>): String = Format.entries.filter { it in off }.joinToString(",") { it.key }
}
