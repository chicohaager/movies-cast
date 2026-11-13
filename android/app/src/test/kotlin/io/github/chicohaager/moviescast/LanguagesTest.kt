package io.github.chicohaager.moviescast

import io.github.chicohaager.moviescast.ui.Languages
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class LanguagesTest {
    @Test
    fun aResourceFolderBecomesALanguageTag() {
        assertEquals("pt-BR", Languages.tag("pt-rBR"))
        assertEquals("zh-TW", Languages.tag("zh-rTW"))
        assertEquals("de", Languages.tag("de"))
    }

    @Test
    fun everyLanguageOfTheBuildIsOfferedUnderItsOwnName() {
        assertEquals(BuildConfig.LANGUAGES.size, Languages.tags.size)
        assertEquals(18, Languages.tags.size)
        val names = Languages.tags.map(Languages::ownName)
        assertEquals("Deutsch", Languages.ownName("de"))
        assertEquals("English", Languages.ownName("en"))
        // Two kinds of Portuguese and of Chinese must not look alike in the list.
        assertEquals(names.size, names.toSet().size)
        for (tag in Languages.tags) {
            // A tag Java does not know comes back as the tag itself or empty.
            assertTrue(tag, Locale.forLanguageTag(tag).language.isNotEmpty())
            assertTrue(tag, Languages.ownName(tag).isNotBlank() && Languages.ownName(tag) != tag)
        }
    }
}
