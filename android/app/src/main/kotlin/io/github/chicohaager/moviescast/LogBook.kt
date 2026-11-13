package io.github.chicohaager.moviescast

/**
 * The player's last log lines, kept in memory for "Share diagnostics": a tester without a PC has
 * no `adb logcat`. Nothing is written to disk, and the lines are gone when the app is closed.
 *
 * Every line passes [scrub] on the way in, so a line that names a server by mistake cannot carry
 * the address out of the device.
 */
class LogBook(private val capacity: Int = 200) {
    private val lines = ArrayDeque<String>()

    fun add(line: String) {
        synchronized(lines) {
            lines.addLast(scrub(line))
            while (lines.size > capacity) lines.removeFirst()
        }
    }

    fun lines(): List<String> = synchronized(lines) { lines.toList() }

    companion object {
        private val url = Regex("""[a-zA-Z][a-zA-Z0-9+.-]*://\S+""")
        private val ipv4 = Regex("""(?<![\d.])\d{1,3}(?:\.\d{1,3}){3}(?::\d+)?(?![\d.])""")
        private const val HIDDEN = "<address>"

        /** A URL or an IPv4 address in the text becomes `<address>`. */
        fun scrub(text: String): String = text.replace(url, HIDDEN).replace(ipv4, HIDDEN)

        /**
         * The text "Share diagnostics" hands out: one `name: value` line per fact, the log lines,
         * and last the device's decoders, a long list that would otherwise push the log off the page.
         */
        fun report(facts: List<Pair<String, String>>, lines: List<String>, decoders: List<String> = emptyList()): String = buildString {
            for ((name, value) in facts) append(name).append(": ").append(scrub(value)).append('\n')
            append('\n').append("player log (").append(lines.size).append(" lines)\n")
            for (line in lines) append(scrub(line)).append('\n')
            if (decoders.isNotEmpty()) {
                append('\n').append("decoders (").append(decoders.size).append(")\n")
                for (decoder in decoders) append(scrub(decoder)).append('\n')
            }
        }
    }
}
