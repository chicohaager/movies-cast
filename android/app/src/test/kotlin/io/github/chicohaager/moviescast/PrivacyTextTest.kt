package io.github.chicohaager.moviescast

import io.github.chicohaager.moviescast.ui.privacyParagraphs
import org.junit.Assert.assertEquals
import org.junit.Test

class PrivacyTextTest {
    @Test
    fun markdownBecomesHeadingsAndPlainParagraphs() {
        val text = "# Privacy\n\nNo `analytics`, see\n[the docs](docs/TECHNICAL.md).\n\n## The app\n\n**Talks to:** one server.\nAsk at <https://example.org/issues>.\n"
        assertEquals(
            listOf(
                true to "Privacy",
                false to "No analytics, see the docs.",
                true to "The app",
                false to "Talks to: one server. Ask at https://example.org/issues.",
            ),
            privacyParagraphs(text),
        )
    }
}
