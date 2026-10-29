/** Search the already-eligible catalog; every word must match, in any order. */
export function searchProducts(products, query) {
  const words = query.normalize("NFKC").toLowerCase().trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return products;
  return products.filter((product) => {
    const text = [product.name, product.id, product.family, product.size, product.pack]
      .filter(Boolean).join(" ").normalize("NFKC").toLowerCase();
    return words.every((word) => text.includes(word));
  });
}
