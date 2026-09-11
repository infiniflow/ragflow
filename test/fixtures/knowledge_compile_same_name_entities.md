# Same-name entities with different categories

This document is a regression fixture for knowledge compilation. The entities
below intentionally use the same name while referring to different categories.
The compiler should preserve the category of each entity and must not create
relations between unrelated entities only because their names are identical.

## Mercury — planet

Mercury is the smallest planet in the Solar System and the planet closest to
the Sun. Mercury completes one orbit around the Sun in about 88 Earth days.
The MESSENGER spacecraft studied Mercury's surface and magnetic field.

Relations:

- Mercury (planet) is closest to the Sun.
- MESSENGER studied Mercury (planet).

## Mercury — chemical element

Mercury is the chemical element with atomic number 80 and the symbol Hg. It is
a metallic element that is liquid at room temperature. Mercury (element) is
used in some scientific instruments, although many applications are limited
because mercury is toxic.

Relations:

- Mercury (element) has atomic number 80.
- Mercury (element) is represented by the symbol Hg.
- Mercury (element) is toxic.

## Mercury — automobile brand

Mercury was an automobile brand created by Ford Motor Company. Mercury cars
were positioned between Ford and Lincoln in Ford's brand portfolio. This
automobile brand is unrelated to both the planet Mercury and the chemical
element mercury.

Relations:

- Ford Motor Company created Mercury (automobile brand).
- Mercury (automobile brand) was positioned between Ford and Lincoln.

## Apple — fruit

An apple is a fruit produced by trees of the genus Malus. Apples are commonly
eaten fresh and can also be used to make juice and cider. This apple is a food
and is unrelated to Apple Inc.

## Apple — technology company

Apple Inc. is a technology company that develops products such as the iPhone,
the iPad, and the Mac. Apple Inc. is headquartered in Cupertino, California.
Apple Inc. is a company and is unrelated to the apple fruit.

Relations:

- Apple Inc. develops the iPhone.
- Apple Inc. develops the Mac.
- Apple Inc. is headquartered in Cupertino, California.

## Expected behavior

When this document is compiled, same-name records should be distinguishable
by their semantic category or context. In particular:

1. The planet Mercury, the chemical element mercury, and the Mercury
   automobile brand must not be merged into an unrelated single entity.
2. The apple fruit and Apple Inc. must not be connected merely because their
   names match.
3. Relations should use the correct entity category as their endpoint.
4. Source chunk IDs and descriptions should remain available after any valid
   same-name merge.
