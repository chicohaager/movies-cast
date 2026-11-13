package io.github.chicohaager.moviescast.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Test

/** Invented values only. */
class SavedLoginsTest {
    private val a = SavedLogin("http://192.0.2.10/movies", "u1", "anna", "sealed-a", "box")
    private val b = SavedLogin("http://192.0.2.20:8096", "u2", "ben", "sealed-b")
    private val c = SavedLogin("http://192.0.2.10/movies", "u3", "cleo", "sealed-c", "box")

    @Test
    fun aNewLoginIsAddedAndPutInUse() {
        val two = SavedLogins().with(a).with(b)
        assertEquals(listOf(a, b), two.logins)
        assertEquals(b, two.active)
    }

    @Test
    fun aSecondAccountAtTheSameServerIsALoginOfItsOwn() {
        val two = SavedLogins().with(a).with(c)
        assertEquals(listOf(a, c), two.logins)
    }

    @Test
    fun theSameAccountAtTheSameServerIsReplacedWhereItStands() {
        val again = a.copy(sealedToken = "sealed-new")
        val list = SavedLogins().with(a).with(b).with(again)
        assertEquals(listOf(again, b), list.logins)
        assertEquals(again, list.active)
    }

    @Test
    fun choosingPutsAnotherLoginInUseAndAnIndexOutsideChangesNothing() {
        val list = SavedLogins().with(a).with(b)
        assertEquals(a, list.chosen(0).active)
        assertEquals(b, list.chosen(7).active)
        assertEquals(b, list.chosen(-1).active)
    }

    @Test
    fun droppingALoginBeforeTheOneInUseKeepsThatOneInUse() {
        val list = SavedLogins().with(a).with(b).with(c).chosen(1).without(0)
        assertEquals(listOf(b, c), list.logins)
        assertEquals(b, list.active)
    }

    @Test
    fun droppingALoginAfterTheOneInUseKeepsThatOneInUse() {
        val list = SavedLogins().with(a).with(b).with(c).chosen(1).without(2)
        assertEquals(listOf(a, b), list.logins)
        assertEquals(b, list.active)
    }

    @Test
    fun droppingTheLoginInUseLeavesNoneInUse() {
        // not the last of the list: an index left standing would name the login after it
        val list = SavedLogins().with(a).with(b).with(c).chosen(1).without(1)
        assertEquals(listOf(a, c), list.logins)
        assertNull(list.active)
        assertEquals(-1, list.current)
    }

    @Test
    fun whatWasWrittenIsReadAgain() {
        val list = SavedLogins().with(a).with(b).chosen(0)
        assertEquals(list, SavedLogins.decode(list.encode()))
    }

    @Test
    fun aTextThatIsNoListGivesAnEmptyOne() {
        assertEquals(SavedLogins(), SavedLogins.decode(null))
        assertEquals(SavedLogins(), SavedLogins.decode("not json"))
        assertEquals(SavedLogins(), SavedLogins.decode("""{"logins":7}"""))
    }

    @Test
    fun anIndexOutsideTheListIsReadAsNoneInUse() {
        val text = SavedLogins(listOf(a), 0).encode().replace("\"current\":0", "\"current\":5")
        assertFalse(text, text.contains("\"current\":0"))
        val read = SavedLogins.decode(text)
        assertEquals(listOf(a), read.logins)
        assertNull(read.active)
        // put right when read, not carried along
        assertEquals(-1, read.current)
    }

    @Test
    fun theTitleNamesTheAccountAndTheServerWhenItHasAName() {
        assertEquals("anna · box", a.title)
        assertEquals("ben", b.title)
    }

    @Test
    fun theTextFormOfALoginDoesNotCarryTheToken() {
        assertFalse(a.toString().contains("sealed-a"))
    }
}
