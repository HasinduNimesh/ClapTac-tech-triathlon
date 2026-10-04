#!/usr/bin/env python3
"""Generate database/seeds/catalog.sql: the product catalog and each store's range.

The product list below is the single source of truth for the seeded catalog.
Store size comes from the official dataset (average weekly order units per
outlet in deliveries_train.csv, relative to its brand), so busier outlets get a
wider range and higher daily sales targets. Output is deterministic.

    python3 scripts/generate-catalog-seed.py
"""

import csv
import hashlib
import statistics
from collections import defaultdict
from datetime import date
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DELIVERIES = ROOT / "Tech-Triathlon 2026 - Datasets" / "data" / "Training Data" / "deliveries_train.csv"
OUTLETS = ROOT / "database" / "seeds" / "outlets.csv"
OUT = ROOT / "database" / "seeds" / "catalog.sql"
PRICE_FROM = date(2026, 1, 1)
PREVIOUS_PRICE_FROM = date(2025, 7, 1)

CATEGORIES = [
    # id, parent, brand, name
    ("FR-PANTRY", None, "Fresh", "Pantry"),
    ("FR-PANTRY-RICE", "FR-PANTRY", "Fresh", "Rice and grains"),
    ("FR-PANTRY-PULSES", "FR-PANTRY", "Fresh", "Pulses, flour and sugar"),
    ("FR-PANTRY-OILS", "FR-PANTRY", "Fresh", "Oils"),
    ("FR-PANTRY-SPICES", "FR-PANTRY", "Fresh", "Spices"),
    ("FR-PANTRY-PACKAGED", "FR-PANTRY", "Fresh", "Packaged food"),
    ("FR-DAIRY", None, "Fresh", "Dairy and chilled"),
    ("FR-MEAT", None, "Fresh", "Meat and fish"),
    ("FR-BEVERAGES", None, "Fresh", "Beverages"),
    ("FR-BAKERY", None, "Fresh", "Bakery and biscuits"),
    ("FR-PRODUCE", None, "Fresh", "Fruit and vegetables"),
    ("FR-HOUSEHOLD", None, "Fresh", "Household"),
    ("ST-MEN", None, "Style", "Men"),
    ("ST-WOMEN", None, "Style", "Women"),
    ("ST-KIDS", None, "Style", "Kids"),
    ("ST-FOOTWEAR", None, "Style", "Footwear"),
    ("ST-ACCESSORIES", None, "Style", "Accessories"),
    ("ST-STORE-SUPPLIES", None, "Style", "Store supplies"),
    ("TC-MOBILE", None, "Tech", "Mobile phones and tablets"),
    ("TC-COMPUTING", None, "Tech", "Computing"),
    ("TC-AUDIO", None, "Tech", "Audio and wearables"),
    ("TC-TV", None, "Tech", "Television"),
    ("TC-HOME", None, "Tech", "Home appliances"),
    ("TC-ACCESSORIES", None, "Tech", "Accessories and storage"),
]

# Fresh: id, category, family, name, size, each, pack, units/pack, pack kg, pack m3, temp,
#        price LKR/each, eaches sold per day at an average store, shelf-life days, perishable, aliases
FRESH = [
    ("FR-RICE-5KG", "FR-PANTRY-RICE", "rice", "Samba rice 5 kg", "5 kg", "bag", "bag", 1, 5.1, 0.009, "ambient", 1450, 6, 365, False, ["rice", "samba", "samba rice"]),
    ("FR-RICE-10KG", "FR-PANTRY-RICE", "rice", "Samba rice 10 kg", "10 kg", "bag", "bag", 1, 10.2, 0.017, "ambient", 2850, 2.5, 365, False, ["rice", "samba"]),
    ("FR-REDRICE-5KG", "FR-PANTRY-RICE", "red rice", "Red raw rice 5 kg", "5 kg", "bag", "bag", 1, 5.1, 0.009, "ambient", 1150, 3, 365, False, ["red rice", "rathu kekulu"]),
    ("FR-KEERI-5KG", "FR-PANTRY-RICE", "keeri samba", "Keeri samba rice 5 kg", "5 kg", "bag", "bag", 1, 5.1, 0.009, "ambient", 1900, 2, 365, False, ["keeri samba"]),
    ("FR-BASMATI-1KG", "FR-PANTRY-RICE", "basmati", "Basmati rice 1 kg", "1 kg", "packet", "bale", 10, 10.3, 0.016, "ambient", 950, 2, 540, False, ["basmati"]),
    ("FR-FLOUR-1KG", "FR-PANTRY-PULSES", "flour", "Wheat flour 1 kg", "1 kg", "packet", "bale", 20, 20.4, 0.03, "ambient", 260, 9, 270, False, ["flour", "wheat flour", "piti"]),
    ("FR-DHAL-1KG", "FR-PANTRY-PULSES", "dhal", "Red dhal 1 kg", "1 kg", "packet", "bale", 20, 20.3, 0.026, "ambient", 420, 8, 365, False, ["dhal", "dal", "lentils", "parippu"]),
    ("FR-CHICKPEA-500G", "FR-PANTRY-PULSES", "chickpeas", "Chickpeas 500 g", "500 g", "packet", "bale", 20, 10.2, 0.015, "ambient", 380, 3, 365, False, ["chickpeas", "kadala"]),
    ("FR-MUNG-500G", "FR-PANTRY-PULSES", "green gram", "Green gram 500 g", "500 g", "packet", "bale", 20, 10.2, 0.015, "ambient", 420, 2, 365, False, ["green gram", "mung", "mung ata"]),
    ("FR-SUGAR-1KG", "FR-PANTRY-PULSES", "sugar", "White sugar 1 kg", "1 kg", "packet", "bale", 20, 20.3, 0.026, "ambient", 290, 10, 540, False, ["sugar", "white sugar", "seeni"]),
    ("FR-BROWNSUGAR-1KG", "FR-PANTRY-PULSES", "brown sugar", "Brown sugar 1 kg", "1 kg", "packet", "bale", 20, 20.3, 0.026, "ambient", 340, 2, 540, False, ["brown sugar"]),
    ("FR-SALT-1KG", "FR-PANTRY-PULSES", "salt", "Table salt 1 kg", "1 kg", "packet", "case", 24, 24.5, 0.028, "ambient", 140, 4, 720, False, ["salt", "lunu"]),
    ("FR-OIL-1L", "FR-PANTRY-OILS", "oil", "Coconut oil 1 L", "1 L", "bottle", "case", 12, 11.6, 0.016, "ambient", 920, 6, 365, False, ["oil", "coconut oil", "pol thel"]),
    ("FR-OIL-500ML", "FR-PANTRY-OILS", "oil", "Coconut oil 500 ml", "500 ml", "bottle", "case", 24, 11.9, 0.017, "ambient", 480, 5, 365, False, ["oil", "coconut oil"]),
    ("FR-VEGOIL-1L", "FR-PANTRY-OILS", "vegetable oil", "Vegetable oil 1 L", "1 L", "bottle", "case", 12, 11.4, 0.016, "ambient", 1050, 2, 365, False, ["vegetable oil", "cooking oil"]),
    ("FR-CURRY-100G", "FR-PANTRY-SPICES", "curry powder", "Roasted curry powder 100 g", "100 g", "packet", "case", 50, 5.4, 0.012, "ambient", 260, 5, 270, False, ["curry powder", "thuna paha"]),
    ("FR-CHILLI-100G", "FR-PANTRY-SPICES", "chilli powder", "Chilli powder 100 g", "100 g", "packet", "case", 50, 5.4, 0.012, "ambient", 280, 5, 270, False, ["chilli powder", "miris kudu"]),
    ("FR-COCOMILK-300G", "FR-PANTRY-PACKAGED", "coconut milk", "Coconut milk powder 300 g", "300 g", "packet", "case", 24, 7.6, 0.016, "ambient", 620, 4, 365, False, ["coconut milk", "coconut milk powder"]),
    ("FR-TINFISH-425G", "FR-PANTRY-PACKAGED", "canned fish", "Canned mackerel 425 g", "425 g", "can", "case", 24, 10.8, 0.012, "ambient", 690, 5, 1095, False, ["canned fish", "tin fish", "mackerel"]),
    ("FR-NOODLES-400G", "FR-PANTRY-PACKAGED", "noodles", "Instant noodles 400 g", "400 g", "packet", "case", 24, 10.1, 0.035, "ambient", 420, 6, 270, False, ["noodles"]),
    ("FR-KITHUL-750ML", "FR-PANTRY-PACKAGED", "treacle", "Kithul treacle 750 ml", "750 ml", "bottle", "case", 12, 12.6, 0.014, "ambient", 1100, 1, 540, False, ["kithul", "treacle", "pani"]),
    ("FR-JAM-500G", "FR-PANTRY-PACKAGED", "jam", "Mixed fruit jam 500 g", "500 g", "jar", "case", 12, 7.9, 0.011, "ambient", 780, 1.5, 540, False, ["jam"]),
    ("FR-EGGS-30", "FR-PANTRY-PACKAGED", "eggs", "Eggs", "tray of 30", "egg", "tray", 30, 1.9, 0.012, "ambient", 55, 60, 21, True, ["eggs", "egg", "biththara"]),
    ("FR-MILK-1L", "FR-DAIRY", "milk", "Fresh milk 1 L", "1 L", "bottle", "crate", 12, 12.9, 0.017, "chilled", 520, 14, 7, True, ["milk", "fresh milk", "kiri"]),
    ("FR-MILK-500ML", "FR-DAIRY", "milk", "Fresh milk 500 ml", "500 ml", "bottle", "crate", 24, 13.0, 0.018, "chilled", 280, 12, 7, True, ["milk", "fresh milk"]),
    ("FR-FLAVMILK-180ML", "FR-DAIRY", "flavoured milk", "Chocolate milk 180 ml", "180 ml", "pack", "crate", 24, 4.6, 0.009, "chilled", 160, 16, 14, True, ["chocolate milk", "flavoured milk"]),
    ("FR-YOGHURT-80G", "FR-DAIRY", "yoghurt", "Yoghurt cup 80 g", "80 g", "cup", "crate", 48, 4.6, 0.018, "chilled", 90, 38, 14, True, ["yoghurt", "yogurt", "curd cups"]),
    ("FR-CURD-1L", "FR-DAIRY", "curd", "Buffalo curd 1 L clay pot", "1 L", "pot", "crate", 6, 8.4, 0.024, "chilled", 650, 4, 5, True, ["curd", "buffalo curd", "meekiri"]),
    ("FR-BUTTER-200G", "FR-DAIRY", "butter", "Butter 200 g", "200 g", "pack", "case", 24, 5.2, 0.008, "chilled", 980, 2, 90, True, ["butter"]),
    ("FR-CHEESE-200G", "FR-DAIRY", "cheese", "Cheese slices 200 g", "200 g", "pack", "case", 24, 5.2, 0.009, "chilled", 1350, 1.5, 90, True, ["cheese", "cheese slices"]),
    ("FR-CREAM-200ML", "FR-DAIRY", "cream", "Fresh cream 200 ml", "200 ml", "pack", "case", 24, 5.1, 0.008, "chilled", 620, 1, 21, True, ["cream", "fresh cream"]),
    ("FR-MARGARINE-250G", "FR-DAIRY", "margarine", "Margarine 250 g", "250 g", "tub", "case", 24, 6.4, 0.011, "chilled", 450, 3, 120, True, ["margarine"]),
    ("FR-SAUSAGE-500G", "FR-MEAT", "sausages", "Chicken sausages 500 g", "500 g", "pack", "case", 12, 6.3, 0.012, "chilled", 1150, 3, 21, True, ["sausages", "chicken sausages"]),
    ("FR-CHICKEN-1KG", "FR-MEAT", "chicken", "Fresh whole chicken 1 kg", "1 kg", "pack", "crate", 10, 10.4, 0.03, "chilled", 1450, 5, 4, True, ["chicken", "whole chicken"]),
    ("FR-FISH-500G", "FR-MEAT", "fish", "Seer fish fillet 500 g", "500 g", "pack", "case", 10, 5.3, 0.012, "chilled", 1600, 2, 3, True, ["fish", "seer fish", "thora"]),
    ("FR-TEA-400G", "FR-BEVERAGES", "tea", "Ceylon black tea 400 g", "400 g", "packet", "case", 24, 10.1, 0.026, "ambient", 1250, 4, 540, False, ["tea", "black tea", "tea leaves"]),
    ("FR-TEABAG-100", "FR-BEVERAGES", "tea bags", "Ceylon tea bags 100s", "100 bags", "box", "case", 24, 5.6, 0.03, "ambient", 900, 2, 540, False, ["tea bags"]),
    ("FR-COFFEE-100G", "FR-BEVERAGES", "coffee", "Instant coffee 100 g", "100 g", "jar", "case", 24, 5.2, 0.014, "ambient", 1150, 1, 540, False, ["coffee", "instant coffee"]),
    ("FR-MALTED-400G", "FR-BEVERAGES", "malted milk", "Malted milk powder 400 g", "400 g", "jar", "case", 24, 10.6, 0.03, "ambient", 980, 2, 365, False, ["malted milk", "milo"]),
    ("FR-WATER-1500ML", "FR-BEVERAGES", "water", "Bottled water 1.5 L", "1.5 L", "bottle", "pack", 12, 18.4, 0.024, "ambient", 150, 18, 365, False, ["water", "bottled water"]),
    ("FR-WATER-500ML", "FR-BEVERAGES", "water", "Bottled water 500 ml", "500 ml", "bottle", "pack", 24, 12.5, 0.017, "ambient", 80, 24, 365, False, ["water", "small water"]),
    ("FR-CORDIAL-750ML", "FR-BEVERAGES", "cordial", "Orange cordial 750 ml", "750 ml", "bottle", "case", 12, 11.2, 0.014, "ambient", 720, 1.5, 365, False, ["cordial", "orange cordial"]),
    ("FR-GINGERBEER-400ML", "FR-BEVERAGES", "ginger beer", "Ginger beer 400 ml", "400 ml", "bottle", "case", 24, 12.1, 0.018, "ambient", 220, 8, 270, False, ["ginger beer", "egb"]),
    ("FR-COLA-1500ML", "FR-BEVERAGES", "cola", "Cola 1.5 L", "1.5 L", "bottle", "pack", 6, 9.3, 0.013, "ambient", 480, 5, 270, False, ["cola", "soft drink"]),
    ("FR-BREAD-450G", "FR-BAKERY", "bread", "Sandwich bread 450 g", "450 g", "loaf", "tray", 10, 4.8, 0.042, "ambient", 190, 26, 3, True, ["bread", "loaf", "paan"]),
    ("FR-ROLLS-6", "FR-BAKERY", "bread rolls", "Bread rolls 6 pack", "6 rolls", "pack", "tray", 12, 4.2, 0.045, "ambient", 260, 8, 3, True, ["bread rolls", "buns"]),
    ("FR-CRACKER-190G", "FR-BAKERY", "crackers", "Cream crackers 190 g", "190 g", "packet", "case", 36, 7.4, 0.035, "ambient", 260, 9, 270, False, ["cream crackers", "crackers"]),
    ("FR-MARIE-80G", "FR-BAKERY", "biscuits", "Marie biscuits 80 g", "80 g", "packet", "case", 48, 4.3, 0.022, "ambient", 90, 14, 270, False, ["marie", "biscuits"]),
    ("FR-CHOCBISC-100G", "FR-BAKERY", "biscuits", "Chocolate biscuits 100 g", "100 g", "packet", "case", 36, 4.0, 0.02, "ambient", 250, 7, 270, False, ["chocolate biscuits", "biscuits"]),
    ("FR-CAKE-500G", "FR-BAKERY", "cake", "Butter cake 500 g", "500 g", "cake", "tray", 8, 4.4, 0.03, "ambient", 850, 2, 7, True, ["cake", "butter cake"]),
    ("FR-BANANA-1KG", "FR-PRODUCE", "bananas", "Kolikuttu bananas 1 kg", "1 kg", "bunch", "crate", 12, 12.6, 0.045, "ambient", 380, 8, 5, True, ["bananas", "banana", "kesel"]),
    ("FR-ONION-1KG", "FR-PRODUCE", "onions", "Big onions 1 kg", "1 kg", "bag", "bag", 20, 20.2, 0.04, "ambient", 420, 10, 30, True, ["onions", "big onions", "lunu"]),
    ("FR-POTATO-1KG", "FR-PRODUCE", "potatoes", "Potatoes 1 kg", "1 kg", "bag", "bag", 20, 20.2, 0.035, "ambient", 380, 9, 30, True, ["potatoes", "potato", "ala"]),
    ("FR-CARROT-500G", "FR-PRODUCE", "carrots", "Carrots 500 g", "500 g", "pack", "crate", 20, 10.3, 0.03, "chilled", 260, 6, 10, True, ["carrots", "carrot"]),
    ("FR-LEEKS-500G", "FR-PRODUCE", "leeks", "Leeks 500 g", "500 g", "pack", "crate", 20, 10.3, 0.035, "chilled", 220, 4, 7, True, ["leeks"]),
    ("FR-TOMATO-500G", "FR-PRODUCE", "tomatoes", "Tomatoes 500 g", "500 g", "pack", "crate", 20, 10.4, 0.03, "ambient", 300, 6, 6, True, ["tomatoes", "tomato", "thakkali"]),
    ("FR-COCONUT", "FR-PRODUCE", "coconuts", "Coconut", "each", "nut", "bag", 25, 15.5, 0.06, "ambient", 160, 15, 30, True, ["coconut", "coconuts", "pol"]),
    ("FR-LIME-250G", "FR-PRODUCE", "limes", "Limes 250 g", "250 g", "pack", "crate", 20, 5.2, 0.02, "ambient", 180, 4, 10, True, ["limes", "lime", "dehi"]),
    ("FR-DISHWASH-500ML", "FR-HOUSEHOLD", "dishwash", "Dishwash liquid 500 ml", "500 ml", "bottle", "case", 12, 6.4, 0.01, "ambient", 420, 2, 720, False, ["dishwash", "dish wash"]),
    ("FR-LAUNDRY-1KG", "FR-HOUSEHOLD", "laundry powder", "Laundry powder 1 kg", "1 kg", "packet", "case", 12, 12.4, 0.025, "ambient", 780, 2, 720, False, ["laundry powder", "washing powder"]),
    ("FR-TISSUE-4", "FR-HOUSEHOLD", "toilet paper", "Toilet paper 4 rolls", "4 rolls", "pack", "pack", 12, 4.9, 0.09, "ambient", 520, 4, 1095, False, ["toilet paper", "tissue"]),
    ("FR-SOAP-100G", "FR-HOUSEHOLD", "soap", "Bath soap 100 g", "100 g", "bar", "case", 72, 7.6, 0.014, "ambient", 160, 8, 1095, False, ["soap", "bath soap"]),
]

# Style: id, category, family, name, each, pack, units/pack, pack kg, pack m3, price, per day, gender, sizes, colours, material, season, aliases
STYLE = [
    ("ST-TSHIRT", "ST-MEN", "t-shirts", "Crew neck T-shirt", "piece", "carton", 40, 9.5, 0.06, 1890, 3, "unisex", "S-XXL", "white, black, navy", "cotton", "all_year", ["t-shirt", "tshirts", "tees"]),
    ("ST-POLO", "ST-MEN", "polo shirts", "Polo shirt", "piece", "carton", 30, 8.4, 0.06, 2990, 1.5, "men", "S-XXL", "navy, maroon, grey", "cotton pique", "all_year", ["polo", "polo shirt"]),
    ("ST-FORMALSHIRT", "ST-MEN", "formal shirts", "Long-sleeve formal shirt", "piece", "carton", 30, 9.0, 0.07, 3990, 1.2, "men", "14.5-17.5", "white, light blue", "poly-cotton", "all_year", ["formal shirt", "office shirt"]),
    ("ST-DENIM", "ST-MEN", "denim", "Slim fit denim jeans", "piece", "carton", 20, 13.0, 0.07, 5490, 1.5, "unisex", "28-38", "indigo, black", "denim", "all_year", ["jeans", "denim"]),
    ("ST-CHINO", "ST-MEN", "chinos", "Chino trousers", "piece", "carton", 20, 11.0, 0.07, 4490, 0.8, "men", "28-38", "khaki, navy", "cotton twill", "all_year", ["chinos"]),
    ("ST-OFFICETROUSER", "ST-MEN", "trousers", "Office trousers", "piece", "carton", 20, 10.5, 0.07, 4290, 0.9, "men", "28-40", "black, charcoal", "polyester blend", "all_year", ["trousers", "office pants"]),
    ("ST-SARONG", "ST-MEN", "sarongs", "Cotton sarong", "piece", "carton", 50, 12.5, 0.06, 1850, 1.2, "men", "free size", "checked, plain", "cotton", "avurudu", ["sarong", "sarama"]),
    ("ST-BATIK", "ST-MEN", "batik shirts", "Batik shirt", "piece", "carton", 30, 7.8, 0.06, 4990, 0.6, "men", "S-XXL", "assorted", "cotton", "festive", ["batik", "batik shirt"]),
    ("ST-BLOUSE", "ST-WOMEN", "blouses", "Women's blouse", "piece", "carton", 40, 8.0, 0.06, 3290, 1.6, "women", "XS-XL", "assorted", "viscose", "all_year", ["blouse", "top"]),
    ("ST-DRESS", "ST-WOMEN", "dresses", "Casual dress", "piece", "carton", 25, 8.8, 0.07, 5990, 1.0, "women", "XS-XL", "floral, plain", "rayon", "all_year", ["dress", "frock"]),
    ("ST-SKIRT", "ST-WOMEN", "skirts", "Midi skirt", "piece", "carton", 30, 7.5, 0.06, 3490, 0.7, "women", "XS-XL", "black, navy, beige", "polyester", "all_year", ["skirt"]),
    ("ST-SAREE", "ST-WOMEN", "sarees", "Printed saree", "piece", "carton", 20, 12.0, 0.07, 9500, 0.4, "women", "free size", "assorted", "georgette", "festive", ["saree", "sari"]),
    ("ST-SHALWAR", "ST-WOMEN", "shalwar", "Shalwar set", "set", "carton", 20, 10.0, 0.07, 6500, 0.5, "women", "S-XL", "assorted", "cotton", "festive", ["shalwar", "shalwar kameez"]),
    ("ST-KIDSTEE", "ST-KIDS", "kids t-shirts", "Kids T-shirt", "piece", "carton", 50, 7.5, 0.06, 1290, 2, "kids", "2-12 years", "assorted", "cotton", "all_year", ["kids t-shirt", "kids tee"]),
    ("ST-UNIFORM", "ST-KIDS", "school uniforms", "School uniform shirt", "piece", "carton", 40, 9.2, 0.06, 1690, 1.2, "kids", "4-16 years", "white", "poly-cotton", "school_term", ["school shirt", "uniform"]),
    ("ST-SHOES", "ST-FOOTWEAR", "shoes", "Black school shoes", "pair", "carton", 12, 10.5, 0.09, 4990, 0.8, "kids", "28-42", "black", "synthetic leather", "school_term", ["shoes", "school shoes"]),
    ("ST-SANDALS", "ST-FOOTWEAR", "sandals", "Sandals", "pair", "carton", 24, 9.6, 0.09, 2490, 1.2, "unisex", "36-45", "brown, black", "synthetic", "all_year", ["sandals"]),
    ("ST-SNEAKERS", "ST-FOOTWEAR", "sneakers", "Canvas sneakers", "pair", "carton", 12, 9.0, 0.1, 7990, 0.6, "unisex", "36-45", "white, black", "canvas", "all_year", ["sneakers", "canvas shoes"]),
    ("ST-SLIPPERS", "ST-FOOTWEAR", "slippers", "Rubber slippers", "pair", "carton", 48, 12.0, 0.09, 790, 3, "unisex", "6-11", "assorted", "rubber", "monsoon", ["slippers", "flip flops"]),
    ("ST-SOCKS-3", "ST-ACCESSORIES", "socks", "Socks 3 pack", "pack", "carton", 100, 8.0, 0.05, 990, 2, "unisex", "free size", "black, white", "cotton", "all_year", ["socks"]),
    ("ST-UNDERWEAR-3", "ST-ACCESSORIES", "underwear", "Cotton briefs 3 pack", "pack", "carton", 100, 9.5, 0.05, 1490, 1.8, "men", "S-XL", "assorted", "cotton", "all_year", ["underwear", "briefs"]),
    ("ST-HANDBAG", "ST-ACCESSORIES", "handbags", "Shoulder handbag", "piece", "carton", 20, 9.0, 0.09, 6990, 0.4, "women", "one size", "black, tan", "faux leather", "festive", ["handbag", "bag"]),
    ("ST-BELT", "ST-ACCESSORIES", "belts", "Leather belt", "piece", "carton", 50, 8.5, 0.04, 1990, 0.8, "men", "30-42", "black, brown", "leather", "all_year", ["belt"]),
    ("ST-CAP", "ST-ACCESSORIES", "caps", "Baseball cap", "piece", "carton", 60, 6.0, 0.08, 1490, 0.7, "unisex", "adjustable", "assorted", "cotton", "all_year", ["cap", "hat"]),
    ("ST-HANGERS", "ST-STORE-SUPPLIES", "hangers", "Hangers", "bundle", "bundle", 100, 6.0, 0.04, 25, 0, "unisex", "", "black", "plastic", "all_year", ["hangers"]),
    ("ST-BAGS", "ST-STORE-SUPPLIES", "shopping bags", "Shopping bags", "bag", "bundle", 500, 7.5, 0.03, 15, 22, "unisex", "", "white", "paper", "all_year", ["bags", "carrier bags", "shopping bags"]),
]

# Tech: id, category, family, name, each, pack, units/pack, pack kg, pack m3, price, per day, serial, high value, seal, warranty, aliases
TECH = [
    ("TC-PHONE", "TC-MOBILE", "smartphones", "Smartphone 6.5-inch 128 GB", "unit", "carton", 10, 4.5, 0.02, 34990, 0.6, True, True, True, 12, ["phones", "mobiles", "smartphone"]),
    ("TC-PHONE-MID", "TC-MOBILE", "smartphones", "Smartphone 6.7-inch 256 GB", "unit", "carton", 10, 4.8, 0.02, 79990, 0.3, True, True, True, 12, ["phones", "smartphone"]),
    ("TC-FEATUREPHONE", "TC-MOBILE", "feature phones", "Dual-SIM feature phone", "unit", "carton", 20, 3.2, 0.02, 6990, 0.4, True, False, False, 6, ["feature phone", "basic phone"]),
    ("TC-TABLET", "TC-MOBILE", "tablets", "Tablet 10-inch 64 GB", "unit", "carton", 10, 6.5, 0.03, 64990, 0.15, True, True, True, 12, ["tablet", "tab"]),
    ("TC-LAPTOP", "TC-COMPUTING", "laptops", "Laptop 14-inch 512 GB SSD", "unit", "carton", 4, 11.0, 0.05, 189000, 0.1, True, True, True, 24, ["laptops", "notebooks", "laptop"]),
    ("TC-LAPTOP-15", "TC-COMPUTING", "laptops", "Laptop 15.6-inch 1 TB SSD", "unit", "carton", 4, 12.5, 0.06, 229000, 0.06, True, True, True, 24, ["laptops", "laptop"]),
    ("TC-ROUTER", "TC-COMPUTING", "routers", "Wi-Fi 6 router", "unit", "carton", 10, 6.0, 0.04, 14990, 0.15, True, False, False, 12, ["router", "wifi router"]),
    ("TC-EARPHONES", "TC-AUDIO", "earphones", "Wired earphones", "unit", "carton", 100, 5.0, 0.03, 1490, 1.5, False, False, False, 3, ["earphones", "headphones"]),
    ("TC-EARBUDS", "TC-AUDIO", "earbuds", "Wireless earbuds", "unit", "carton", 50, 4.0, 0.03, 8990, 0.6, True, False, False, 6, ["earbuds", "wireless earbuds"]),
    ("TC-SPEAKER", "TC-AUDIO", "speakers", "Bluetooth speaker", "unit", "carton", 20, 9.0, 0.05, 12990, 0.25, True, False, False, 12, ["speaker", "bluetooth speaker"]),
    ("TC-WATCH", "TC-AUDIO", "smartwatches", "Smartwatch", "unit", "carton", 20, 3.0, 0.02, 18990, 0.2, True, True, True, 12, ["smartwatch", "watch"]),
    ("TC-TV-43", "TC-TV", "televisions", "LED TV 43-inch", "unit", "box", 1, 9.5, 0.12, 139000, 0.05, True, True, True, 24, ["tv", "television", "43 inch tv"]),
    ("TC-TV-32", "TC-TV", "televisions", "LED TV 32-inch", "unit", "box", 1, 5.5, 0.07, 74990, 0.08, True, True, True, 24, ["tv", "television", "32 inch tv"]),
    ("TC-RICECOOKER", "TC-HOME", "rice cookers", "Rice cooker 1.8 L", "unit", "carton", 4, 12.0, 0.08, 15990, 0.15, True, False, False, 12, ["rice cooker"]),
    ("TC-KETTLE", "TC-HOME", "kettles", "Electric kettle 1.7 L", "unit", "carton", 6, 8.0, 0.07, 7990, 0.2, True, False, False, 12, ["kettle", "electric kettle"]),
    ("TC-BLENDER", "TC-HOME", "blenders", "Blender 1.5 L", "unit", "carton", 4, 12.0, 0.08, 18990, 0.1, True, False, False, 12, ["blender", "mixie"]),
    ("TC-IRON", "TC-HOME", "irons", "Steam iron", "unit", "carton", 10, 12.0, 0.06, 6490, 0.15, True, False, False, 12, ["iron", "steam iron"]),
    ("TC-CHARGER", "TC-ACCESSORIES", "chargers", "Phone charger 20 W USB-C", "unit", "carton", 50, 6.0, 0.03, 3490, 1.2, False, False, False, 6, ["chargers", "adapters", "charger"]),
    ("TC-CABLE", "TC-ACCESSORIES", "cables", "USB-C cable 1 m", "unit", "carton", 100, 4.0, 0.02, 1290, 2, False, False, False, 3, ["cables", "usb", "usb cable"]),
    ("TC-POWERBANK", "TC-ACCESSORIES", "power banks", "Power bank 10,000 mAh", "unit", "carton", 40, 10.0, 0.03, 6990, 0.5, True, False, False, 6, ["power bank", "powerbank"]),
    ("TC-EXTENSION", "TC-ACCESSORIES", "extension cords", "Extension cord 4-way 3 m", "unit", "carton", 20, 9.0, 0.05, 3990, 0.4, False, False, False, 12, ["extension cord", "extension"]),
    ("TC-BULB", "TC-ACCESSORIES", "bulbs", "LED bulb 9 W", "unit", "carton", 100, 6.0, 0.06, 690, 2.5, False, False, False, 12, ["bulb", "led bulb"]),
    ("TC-CASE", "TC-ACCESSORIES", "phone cases", "Phone case", "unit", "carton", 100, 4.0, 0.03, 1990, 1.2, False, False, False, 0, ["phone case", "cover"]),
    ("TC-SCREENGUARD", "TC-ACCESSORIES", "screen protectors", "Tempered glass screen protector", "unit", "carton", 200, 3.0, 0.02, 990, 1.5, False, False, False, 0, ["screen protector", "tempered glass"]),
    ("TC-SDCARD", "TC-ACCESSORIES", "memory cards", "microSD card 64 GB", "unit", "carton", 200, 1.5, 0.01, 3290, 0.5, False, False, False, 12, ["memory card", "sd card"]),
    ("TC-FLASH", "TC-ACCESSORIES", "flash drives", "USB flash drive 64 GB", "unit", "carton", 200, 2.0, 0.01, 2990, 0.4, False, False, False, 12, ["flash drive", "pen drive", "usb drive"]),
]

# Always stocked by every store of the brand (the rest of the range depends on store size).
CORE = {
    "FR-RICE-5KG", "FR-DHAL-1KG", "FR-SUGAR-1KG", "FR-FLOUR-1KG", "FR-OIL-1L", "FR-MILK-1L", "FR-MILK-500ML", "FR-YOGHURT-80G",
    "FR-BREAD-450G", "FR-EGGS-30", "FR-WATER-1500ML", "FR-TEA-400G", "FR-BUTTER-200G", "FR-CHEESE-200G", "FR-ONION-1KG", "FR-COCONUT",
    "ST-TSHIRT", "ST-DENIM", "ST-SHOES", "ST-HANGERS", "ST-BAGS", "ST-SLIPPERS",
    "TC-PHONE", "TC-CHARGER", "TC-CABLE", "TC-EARPHONES", "TC-LAPTOP", "TC-CASE",
}


def gtin13(body12: str) -> str:
    total = sum(int(d) * (3 if i % 2 else 1) for i, d in enumerate(body12))
    return body12 + str((10 - total % 10) % 10)


def unit_hash(*parts) -> float:
    digest = hashlib.sha256("|".join(map(str, parts)).encode()).hexdigest()
    return int(digest[:8], 16) / 0xFFFFFFFF


def q(value) -> str:
    if value is None:
        return "NULL"
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return repr(value)
    return "'" + str(value).replace("'", "''") + "'"


def outlet_scale() -> dict[str, float]:
    """Store size relative to its brand: average weekly order units in the official dataset."""
    brand, units, dates = {}, defaultdict(int), set()
    with DELIVERIES.open(encoding="utf-8") as fh:
        for row in csv.DictReader(fh):
            brand[row["outlet_id"]] = row["brand"]
            units[row["outlet_id"]] += int(row["order_units"])
            dates.add(row["order_date"])
    weeks = len(dates) / 7
    weekly = {o: units[o] / weeks for o in units}
    medians = {b: statistics.median(w for o, w in weekly.items() if brand[o] == b) for b in set(brand.values())}
    return {o: round(min(2.0, max(0.5, weekly[o] / medians[brand[o]])), 3) for o in weekly}


def products():
    out = []
    for p in FRESH:
        pid, cat, fam, name, size, each, pack, upp, kg, m3, temp, price, rate, shelf, perish, aliases = p
        storage = (1, 6) if temp == "chilled" else (None, None)
        out.append(dict(id=pid, brand="Fresh", category=cat, family=fam, name=name, size=size, each=each, pack=pack, upp=upp, kg=kg, m3=m3,
                        temp=temp, price=price, rate=rate, aliases=aliases,
                        ext=("product_fresh", ["perishable", "shelf_life_days", "storage_min_c", "storage_max_c"], [perish, shelf, *storage])))
    for p in STYLE:
        pid, cat, fam, name, each, pack, upp, kg, m3, price, rate, gender, sizes, colours, material, season, aliases = p
        out.append(dict(id=pid, brand="Style", category=cat, family=fam, name=name, size=sizes, each=each, pack=pack, upp=upp, kg=kg, m3=m3,
                        temp="ambient", price=price, rate=rate, aliases=aliases,
                        ext=("product_style", ["gender", "size_range", "colours", "material", "season"], [gender, sizes, colours, material, season])))
    for p in TECH:
        pid, cat, fam, name, each, pack, upp, kg, m3, price, rate, serial, high, seal, warranty, aliases = p
        out.append(dict(id=pid, brand="Tech", category=cat, family=fam, name=name, size="", each=each, pack=pack, upp=upp, kg=kg, m3=m3,
                        temp="ambient", price=price, rate=rate, aliases=aliases,
                        ext=("product_tech", ["requires_serial", "high_value", "seal_required", "warranty_months"], [serial, high, seal, warranty])))
    return out


def main() -> None:
    items = products()
    ids = [p["id"] for p in items]
    assert len(ids) == len(set(ids)), "duplicate product id"
    assert CORE <= set(ids), "core product missing"
    scale = outlet_scale()
    with OUTLETS.open(encoding="utf-8") as fh:
        outlets = [(r["outlet_id"], r["brand"]) for r in csv.DictReader(fh)]

    cats, prods, exts, aliases, prices, ranges = [], [], defaultdict(list), [], [], []
    for i, (cid, parent, brand, name) in enumerate(CATEGORIES):
        cats.append(f"({q(cid)}, {q(parent)}, {q(brand)}, {q(name)}, {i})")

    seq = defaultdict(int)
    brand_digit = {"Fresh": "1", "Style": "2", "Tech": "3"}
    ext_cols = {}
    for p in items:
        seq[p["brand"]] += 1
        n = seq[p["brand"]]
        sku = f"WP-{p['id'][:2]}-{n:05d}"
        gtin = gtin13("2" + brand_digit[p["brand"]] + "47" + f"{n:08d}")  # in-store (restricted) GTIN range
        launched = date(2023 + int(unit_hash(p["id"], "launch") * 3), 1 + int(unit_hash(p["id"], "m") * 12), 1)
        prods.append(f"({q(p['id'])}, {q(sku)}, {q(gtin)}, {q(p['brand'])}, {q(p['category'])}, {q(p['family'])}, {q(p['name'])}, {q(p['size'])}, "
                     f"{q(p['each'])}, {q(p['pack'])}, {p['upp']}, {p['kg']}, {p['m3']}, {q(p['temp'])}, {q(launched.isoformat())})")
        table, cols, values = p["ext"]
        ext_cols[table] = cols
        exts[table].append(f"({q(p['id'])}, {', '.join(q(v) for v in values)})")
        for alias in sorted(set(a.lower() for a in p["aliases"] + [p["family"]])):
            aliases.append(f"({q(p['id'])}, {q(alias)})")
        previous = round(p["price"] * (0.92 + unit_hash(p["id"], "price") * 0.06), -1 if p["price"] >= 100 else 0)
        prices.append(f"({q(p['id'])}, {previous}, {q(PREVIOUS_PRICE_FROM.isoformat())}, {q(PRICE_FROM.isoformat())})")
        prices.append(f"({q(p['id'])}, {p['price']}, {q(PRICE_FROM.isoformat())}, NULL)")

    for outlet, brand in outlets:
        size = scale.get(outlet, 1.0)
        for p in (x for x in items if x["brand"] == brand):
            # Bigger stores carry more of the range; core lines are always listed.
            listed = p["id"] in CORE or unit_hash(outlet, p["id"], "range") < min(0.95, 0.45 + 0.35 * size)
            if not listed:
                continue
            daily = round(p["rate"] * size * (0.75 + 0.5 * unit_hash(outlet, p["id"], "rate")), 2)
            cover = 3 if p["temp"] == "chilled" or p["id"] in {"FR-BREAD-450G", "FR-ROLLS-6", "FR-EGGS-30", "FR-CAKE-500G"} else 10
            minimum = max(1, round(daily)) if daily else 0
            reorder = max(minimum, round(daily * cover * 0.6)) if daily else 0
            maximum = max(reorder + p["upp"], round(daily * cover * 1.5)) if daily else p["upp"] * 2
            since = date(2024, 1 + int(unit_hash(outlet, p["id"], "since") * 12), 1)
            ranges.append(f"({q(outlet)}, {q(p['id'])}, {q(since.isoformat())}, {daily}, {reorder}, {minimum}, {maximum})")

    def block(head, rows, tail):
        return [head, ",\n".join(rows), tail]

    lines = [
        "-- Generated by scripts/generate-catalog-seed.py. Do not edit by hand.",
        f"-- {len(CATEGORIES)} categories, {len(items)} products, {len(ranges)} store range rows for {len(outlets)} outlets.",
        "BEGIN;",
        *block("INSERT INTO shared.product_categories (id, parent_id, brand, name, sort_order) VALUES", cats,
               "ON CONFLICT (id) DO UPDATE SET parent_id = EXCLUDED.parent_id, brand = EXCLUDED.brand, name = EXCLUDED.name, sort_order = EXCLUDED.sort_order;"),
        *block("INSERT INTO shared.products (id, sku, gtin, brand, category_id, family, name, size, each_name, pack_name, units_per_pack, pack_weight_kg, pack_volume_m3, temperature, launched_on) VALUES", prods,
               "ON CONFLICT (id) DO UPDATE SET sku = EXCLUDED.sku, gtin = EXCLUDED.gtin, category_id = EXCLUDED.category_id, family = EXCLUDED.family, name = EXCLUDED.name, "
               "size = EXCLUDED.size, each_name = EXCLUDED.each_name, pack_name = EXCLUDED.pack_name, units_per_pack = EXCLUDED.units_per_pack, "
               "pack_weight_kg = EXCLUDED.pack_weight_kg, pack_volume_m3 = EXCLUDED.pack_volume_m3, temperature = EXCLUDED.temperature, updated_at = now();"),
    ]
    for table, rows in exts.items():
        cols = ext_cols[table]
        lines += block(f"INSERT INTO shared.{table} (product_id, {', '.join(cols)}) VALUES", rows,
                       "ON CONFLICT (product_id) DO UPDATE SET " + ", ".join(f"{c} = EXCLUDED.{c}" for c in cols) + ";")
    lines += block("INSERT INTO shared.product_aliases (product_id, alias) VALUES", aliases, "ON CONFLICT DO NOTHING;")
    lines += block("INSERT INTO shared.product_prices (product_id, unit_price, effective_from, effective_to) VALUES", prices,
                   "ON CONFLICT (product_id, price_list, effective_from) DO UPDATE SET unit_price = EXCLUDED.unit_price, effective_to = EXCLUDED.effective_to;")
    lines += block("INSERT INTO shared.outlet_products (outlet_id, product_id, listed_since, avg_daily_sales_each, reorder_point_each, min_stock_each, max_stock_each) VALUES", ranges,
                   "ON CONFLICT (outlet_id, product_id) DO UPDATE SET listed_since = EXCLUDED.listed_since, avg_daily_sales_each = EXCLUDED.avg_daily_sales_each, "
                   "reorder_point_each = EXCLUDED.reorder_point_each, min_stock_each = EXCLUDED.min_stock_each, max_stock_each = EXCLUDED.max_stock_each, updated_at = now();")
    lines.append("COMMIT;")
    OUT.write_text("\n".join(lines) + "\n", encoding="utf-8", newline="\n")
    print(f"wrote {OUT.relative_to(ROOT)}: {len(items)} products, {len(ranges)} store range rows")


if __name__ == "__main__":
    main()
